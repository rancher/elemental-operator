package backend

import "net"

// Socket returns a fake TPM interface from a unix socket
func Socket(f string) (*FakeTPM, error) {
	conn, err := net.Dial("unix", f)
	if err != nil {
		return nil, err
	}
	return &FakeTPM{
		ReadWriteCloser: conn,
	}, nil
}
