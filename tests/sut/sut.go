package sut

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bramvdbogaerde/go-scp"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

type SUT struct {
	Host     string
	Username string
	Password string
	Timeout  int
}

func NewSUT() *SUT {

	user := os.Getenv("TEST_USER")
	if user == "" {
		user = "vagrant"
	}
	pass := os.Getenv("TEST_PASS")
	if pass == "" {
		pass = "vagrant"
	}

	host := os.Getenv("TEST_HOST")
	if host == "" {
		host = "127.0.0.1:2222"
	}

	var timeout = 180
	valueStr := os.Getenv("COS_TIMEOUT")
	value, err := strconv.Atoi(valueStr)
	if err == nil {
		timeout = value
	}

	return &SUT{
		Host:     host,
		Username: user,
		Password: pass,
		Timeout:  timeout,
	}
}

func (s *SUT) EventuallyConnects(t ...int) {
	dur := s.Timeout
	if len(t) > 0 {
		dur = t[0]
	}
	Eventually(func() error {
		out, err := s.command("echo ping", true)
		if out == "ping\n" {
			return nil
		}
		return err
	}, time.Duration(dur)*time.Second, 5*time.Second).ShouldNot(HaveOccurred())
}

// Command sends a command to the SUIT and waits for reply
func (s *SUT) Command(cmd string) (string, error) {
	return s.command(cmd, false)
}

func (s *SUT) command(cmd string, timeout bool) (string, error) {
	client, err := s.connectToHost(timeout)
	if err != nil {
		return "", err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", err
	}

	out, err := session.CombinedOutput(cmd)
	if err != nil {
		return string(out), errors.Wrap(err, string(out))
	}

	return string(out), err
}

func (s *SUT) clientConfig() *ssh.ClientConfig {
	sshConfig := &ssh.ClientConfig{
		User:    s.Username,
		Auth:    []ssh.AuthMethod{ssh.Password(s.Password)},
		Timeout: 30 * time.Second, // max time to establish connection
	}
	sshConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()

	return sshConfig
}

func (s *SUT) SendFile(src, dst, permission string) error {
	sshConfig := s.clientConfig()
	scpClient := scp.NewClientWithTimeout(s.Host, sshConfig, 10*time.Second)
	defer scpClient.Close()

	if err := scpClient.Connect(); err != nil {
		return err
	}

	f, err := os.Open(src)
	if err != nil {
		return err
	}

	defer scpClient.Close()
	defer f.Close()

	if err := scpClient.CopyFile(f, dst, permission); err != nil {
		return err
	}
	return nil
}

func (s *SUT) connectToHost(timeout bool) (*ssh.Client, error) {
	sshConfig := s.clientConfig()

	client, err := DialWithDeadline("tcp", s.Host, sshConfig, timeout)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// GatherAllLogs will try to gather as much info from the system as possible, including services, dmesg and os related info
func (s SUT) GatherAllLogs(services []string, logFiles []string) {
	// services
	for _, ser := range services {
		out, err := s.command(fmt.Sprintf("journalctl -u %s -o short-iso >> /tmp/%s.log", ser, ser), true)
		if err != nil {
			fmt.Printf("Error getting journal for service %s: %s\n", ser, err.Error())
			fmt.Printf("Output from command: %s\n", out)
		}
		s.GatherLog(fmt.Sprintf("/tmp/%s.log", ser))
	}

	// log files
	for _, file := range logFiles {
		s.GatherLog(file)
	}

	// dmesg
	out, err := s.command("dmesg > /tmp/dmesg", true)
	if err != nil {
		fmt.Printf("Error getting dmesg : %s\n", err.Error())
		fmt.Printf("Output from command: %s\n", out)
	}
	s.GatherLog("/tmp/dmesg")

	// grab full journal
	out, err = s.command("journalctl -o short-iso > /tmp/journal.log", true)
	if err != nil {
		fmt.Printf("Error getting full journalctl info : %s\n", err.Error())
		fmt.Printf("Output from command: %s\n", out)
	}
	s.GatherLog("/tmp/journal.log")

	// uname
	out, err = s.command("uname -a > /tmp/uname.log", true)
	if err != nil {
		fmt.Printf("Error getting uname info : %s\n", err.Error())
		fmt.Printf("Output from command: %s\n", out)
	}
	s.GatherLog("/tmp/uname.log")

	// disk info
	out, err = s.command("lsblk -a >> /tmp/disks.log", true)
	if err != nil {
		fmt.Printf("Error getting disk info : %s\n", err.Error())
		fmt.Printf("Output from command: %s\n", out)
	}
	out, err = s.command("blkid >> /tmp/disks.log", true)
	if err != nil {
		fmt.Printf("Error getting disk info : %s\n", err.Error())
		fmt.Printf("Output from command: %s\n", out)
	}
	s.GatherLog("/tmp/disks.log")

	// Grab users
	s.GatherLog("/etc/passwd")
	// Grab system info
	s.GatherLog("/etc/os-release")

}

// GatherLog will try to scp the given log from the machine to a local file
func (s SUT) GatherLog(logPath string) {
	fmt.Printf("Trying to get file: %s\n", logPath)
	sshConfig := s.clientConfig()
	scpClient := scp.NewClientWithTimeout(s.Host, sshConfig, 10*time.Second)

	err := scpClient.Connect()
	if err != nil {
		scpClient.Close()
		fmt.Println("Couldn't establish a connection to the remote server ", err)
		return
	}

	fmt.Printf("Connection to %s established!\n", s.Host)
	baseName := filepath.Base(logPath)
	_ = os.Mkdir("logs", 0755)

	f, _ := os.Create(fmt.Sprintf("logs/%s", baseName))
	// Close the file after it has been copied
	// Close client connection after the file has been copied
	defer scpClient.Close()
	defer f.Close()

	err = scpClient.CopyFromRemote(f, logPath)

	if err != nil {
		fmt.Printf("Error while copying file: %s\n", err.Error())
		return
	}
	// Change perms so its world readable
	_ = os.Chmod(fmt.Sprintf("logs/%s", baseName), 0666)
	fmt.Printf("File %s copied!\n", baseName)

}

// DialWithDeadline Dials SSH with a deadline to avoid Read timeouts
func DialWithDeadline(network string, addr string, config *ssh.ClientConfig, timeout bool) (*ssh.Client, error) {
	conn, err := net.DialTimeout(network, addr, config.Timeout)
	if err != nil {
		return nil, err
	}
	if config.Timeout > 0 {
		conn.SetReadDeadline(time.Now().Add(config.Timeout))
		conn.SetWriteDeadline(time.Now().Add(config.Timeout))
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		return nil, err
	}
	if !timeout {
		conn.SetReadDeadline(time.Time{})
		conn.SetWriteDeadline(time.Time{})
	}

	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for range t.C {
			_, _, err := c.SendRequest("keepalive@golang.org", true, nil)
			if err != nil {
				return
			}
		}
	}()
	return ssh.NewClient(c, chans, reqs), nil
}

func (s *SUT) WriteInlineFile(content, path string) {
	_, err := s.Command(`cat << EOF > ` + path + `
` + content + `
EOF`)
	Expect(err).ToNot(HaveOccurred())
}

func (s *SUT) HasRunningPodByAppLabel(namespace string, applabel string) bool {
	// Get out and use kubectl to get the pod in the proper namespace
	cmd := strings.Join([]string{"k3s", "kubectl", "get", "pod", "-n", namespace, ""}, " ")
	s.Command(cmd)
	return true
}
