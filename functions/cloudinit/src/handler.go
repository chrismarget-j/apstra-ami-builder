package cloudinit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"
)

const (
	apstraDefaultUser     = "admin"
	apstraDefaultPassword = "admin"

	envVarSkipShutdown = "DEBUG_SKIP_SHUTDOWN"

	sshRetryInterval   = time.Second
	sshRetriesMax      = 30
	sshTimeoutInterval = 3 * time.Second

	MessagePing      = "ping"
	MessagePong      = "pong"
	MessageSuccess   = "success"
	OperationInstall = "install"

	cmdSudoCommand    = "sudo -S DEBIAN_FRONTEND=noninteractive sh -c '%s'"
	cmdSudoCommandSep = " && "

	cmdUpdate           = "apt-get update"
	cmdInstallCloudInit = "apt-get install cloud-init --no-install-recommends -y"
	cmdClearSshKeys     = "rm /etc/ssh/ssh_host_*"
	cmdFirewallBlock    = "iptables -I INPUT 2 -i eth0 -j DROP"
	cmdFirewallSave     = "netfilter-persistent save"
	cmdPrintFlag        = "echo \"flag is %s\""
	cmdShutdown         = "shutdown"
)

type Request struct {
	InstanceIp string `json:"instance_ip"`
	Operation  string `json:"operation"`
}

type Response struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func HandleRequest(ctx context.Context, request *Request) (*Response, error) {
	if request == nil {
		err := errors.New("error: nil Request")
		return &Response{
			Error: err.Error(),
		}, err
	}

	dump, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling request - %w", err)
	}

	log.Printf("request received: '%s'", string(dump))

	switch request.Operation {
	case MessagePing:
		log.Printf("operation '%s', responsing '%s'", MessagePing, MessagePong)
		return &Response{
			Message: MessagePong,
		}, nil
	case OperationInstall:
		log.Printf("operation '%s', invoking handler", OperationInstall)
		handler := newHandler(request)
		return handler.handle()
	default:
		err := fmt.Errorf("unknown operation: '%s'", request.Operation)
		return &Response{Message: err.Error()}, err
	}
}

func newHandler(request *Request) *handler {
	h := handler{
		request: request,
	}
	return &h
}

type handler struct {
	request *Request
	target  net.IP
}

func (o *handler) handle() (*Response, error) {
	err := o.parse()
	if err != nil {
		return &Response{Error: err.Error()}, err
	}

	// create the ssh session
	sshSession, err := o.sshSession()
	if err != nil {
		return &Response{Error: err.Error()}, err
	}
	defer sshSession.Close()
	flag := randString(12) // random string printed to stdout after important work is done
	flagChan := make(chan error, 1)

	// use StdoutPipe() for stdout so that we can tee the ssh session stdout to
	// both os.Stdout (lambda log) and to the flag scanner / error detection.
	sshStdoutPipe, err := sshSession.StdoutPipe()
	if err != nil {
		return &Response{Error: err.Error()}, err
	}
	sshStdoutTee := io.TeeReader(sshStdoutPipe, os.Stdout)

	// read stdout, watch for the flag
	go func() {
		var flagFound bool
		stdoutScanner := bufio.NewScanner(sshStdoutTee)
		for stdoutScanner.Scan() {
			if !flagFound && strings.Contains(stdoutScanner.Text(), flag) {
				flagFound = true
			}
		}
		if flagFound {
			flagChan <- stdoutScanner.Err()
		} else {
			err = stdoutScanner.Err()
			if err == nil {
				flagChan <- fmt.Errorf("flag '%s' not found in ssh stdout", flag)
			} else {
				flagChan <- fmt.Errorf("flag '%s' not found in ssh stdout and scanner reported error - %w", flag, err)
			}
		}
		close(flagChan)
	}()

	// hook up ssh stderr to os.Stderr (lambda log)
	sshSession.Stderr = os.Stderr

	cmd := sessionCommand(flag)
	fmt.Printf("sending command via ssh: %s\n", cmd)
	sshSession.Stdin = bytes.NewReader([]byte(apstraDefaultPassword + "\n"))
	err = sshSession.Run(cmd)
	if err != nil {
		return &Response{Error: err.Error()}, err
	}

	err = <-flagChan
	if err != nil {
		err = fmt.Errorf("error looking for flag '%s' in stdout - %w", flag, err)
		return &Response{Error: err.Error()}, err
	}

	return &Response{Message: MessageSuccess}, nil
}

func (o *handler) parse() error {
	o.target = net.ParseIP(o.request.InstanceIp)
	if o.target == nil {
		return fmt.Errorf("failed to parse IP address '%s'", o.request.InstanceIp)
	}
	return nil
}

func (o *handler) sshSession() (*ssh.Session, error) {
	sshConfig := &ssh.ClientConfig{
		User: apstraDefaultUser,
		Auth: []ssh.AuthMethod{ssh.Password(apstraDefaultPassword)},
		// this should be reasonable when run within an AWS VPC because, for example
		// https://kevin.burke.dev/kevin/aws-alb-validation-tls-reply/
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         sshTimeoutInterval,
	}

	var c *ssh.Client
	var err error
	var i int
	for i < sshRetriesMax {
		i++
		c, err = ssh.Dial("tcp", net.JoinHostPort(o.target.String(), "22"), sshConfig)
		if err == nil {
			break
		} else {
			log.Printf("error in ssh.Dial - %s", err.Error())
		}
		time.Sleep(sshRetryInterval)
	}
	if err != nil {
		return nil, fmt.Errorf("error in ssh.Dial - %w", err)
	}

	return c.NewSession()
}

func sessionCommand(flag string) string {
	cmds := []string{
		cmdUpdate,
		cmdInstallCloudInit,
		cmdClearSshKeys,
		cmdFirewallBlock,
		cmdFirewallSave,
		fmt.Sprintf(cmdPrintFlag, flag),
	}

	skipShutdown := os.Getenv(envVarSkipShutdown)

	if skipShutdown != "true" {
		cmds = append(cmds, cmdShutdown)
	}

	return fmt.Sprintf(cmdSudoCommand, strings.Join(cmds, cmdSudoCommandSep))
}

func randString(n int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, n)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}
