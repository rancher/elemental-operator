package sut

import (
	"bytes"
	"crypto/tls"
	"io"
	"net/http"
	"time"
)

func Get(url string) (string, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b := bytes.NewBuffer([]byte{})
	_, err = io.Copy(b, resp.Body)
	if err != nil {
		return "", err
	}
	return b.String(), nil
}
