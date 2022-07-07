package display_cmd

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func NewDisplayCommand() *cobra.Command {
	var path string

	cmd := &cobra.Command{
		Use:   "display",
		Short: "Write the given file to standard out.",
		Run: func(_ *cobra.Command, _ []string) {
			displayRun(path)
		},
	}

	viper.AutomaticEnv()
	cmd.PersistentFlags().StringVar(&path, "file", "", "path to the file to write to standard out")
	_ = viper.BindPFlag("file", cmd.PersistentFlags().Lookup("file"))
	_ = cobra.MarkFlagRequired(cmd.PersistentFlags(), "file")

	return cmd
}

func displayRun(path string) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		<-sigc
		os.Exit(0)
	}()

	f, err := os.Open(path)
	if err != nil {
		logrus.Fatal(err)
	}

	_, err = io.Copy(os.Stdout, f)
	_ = f.Close()
	if err != nil {
		logrus.Fatal(err)
	}

	for {
		time.Sleep(time.Duration(int64(^uint64(0) >> 1)))
	}
}
