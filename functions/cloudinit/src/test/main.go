package main

import (
	"bytes"
	"golang.org/x/crypto/ssh"
	"log"
	"net"
)

const (
	apstraDefaultUser      = "admin"
	apstraDefaultPasswdord = "admin"
)

func main() {
	sshConfig := &ssh.ClientConfig{
		User: apstraDefaultUser,
		Auth: []ssh.AuthMethod{ssh.Password(apstraDefaultPasswdord)},
		// this should be reasonable when run within an AWS VPC becase, for example
		// https://kevin.burke.dev/kevin/aws-alb-validation-tls-reply/
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	sshClient, err := ssh.Dial("tcp", net.JoinHostPort("3.231.152.130", "22"), sshConfig)
	if err != nil {
		log.Fatal(err)
	}

	sshSession, err := sshClient.NewSession()
	if err != nil {
		log.Fatal(err)
	}
	defer sshSession.Close()

	var b bytes.Buffer
	sshSession.Stdout = &b

	// Finally, run the command
	err = sshSession.Run("ls -la")
	log.Print(b.String())

}
