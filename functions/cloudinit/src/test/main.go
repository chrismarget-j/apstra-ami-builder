package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"golang.org/x/crypto/ssh"
	"log"
	"math/rand"
	"net"
	"strings"
	"time"
)

const (
	apstraDefaultUser     = "admin"
	apstraDefaultPassword = "admin"

	cmdSudoCommand    = "sudo -S DEBIAN_FRONTEND=noninteractive sh -c '%s'"
	cmdSudoCommandSep = " && "

	cmdUpdate           = "apt-get update"
	cmdInstallCloudInit = "apt-get install cloud-init --no-install-recommends -y"
	cmdClearSshKeys     = "rm /etc/ssh/ssh_host_*"
	cmdShutdown         = "shutdown now"

	flagString = "flag is \"%s\""
)

func main() {
	sshConfig := &ssh.ClientConfig{
		User: apstraDefaultUser,
		Auth: []ssh.AuthMethod{ssh.Password(apstraDefaultPassword)},
		// this should be reasonable when run within an AWS VPC becase, for example
		// https://kevin.burke.dev/kevin/aws-alb-validation-tls-reply/
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	sshClient, err := ssh.Dial("tcp", net.JoinHostPort("44.193.212.99", "22"), sshConfig)
	if err != nil {
		log.Fatal(err)
	}

	sshSession, err := sshClient.NewSession()
	if err != nil {
		log.Fatal(err)
	}
	defer sshSession.Close()

	stdoutPipe, err := sshSession.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	stdoutScanner := bufio.NewScanner(stdoutPipe)

	stderrPipe, err := sshSession.StderrPipe()
	if err != nil {
		log.Fatal(err)
	}
	stderrScanner := bufio.NewScanner(stderrPipe)

	flag := randString(12)
	cmd := sessionCommand(false, aws.String(flag))
	fmt.Printf("sending command via ssh: %s\n", cmd)
	sshSession.Stdin = bytes.NewReader([]byte(apstraDefaultPassword + "\n"))
	err = sshSession.Run(cmd)
	if err != nil {
		log.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	for stdoutScanner.Scan() {
		stdout.WriteString("  " + stdoutScanner.Text() + "\n")
	}
	for stderrScanner.Scan() {
		stderr.WriteString("  " + stderrScanner.Text() + "\n")
	}

	if !strings.Contains(stderrScanner.Text(), flag) {
		log.Fatal(fmt.Errorf("expected to find '%s' in the final line of stdout", flag))
	}

	//bufio.NewScanner(sshSession.Stdout)
	fmt.Printf("stdout:\n%s", stdout.String())
	fmt.Printf("stderr:\n%s", stderr.String())
}

func sessionCommand(shutdown bool, flag *string) string {
	cmds := []string{
		cmdUpdate,
		cmdInstallCloudInit,
		cmdClearSshKeys,
	}

	if flag != nil {
		cmds = append(cmds, fmt.Sprintf(flagString, *flag))
	}

	if shutdown {
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
