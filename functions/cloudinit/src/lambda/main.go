package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/chrismarget-j/apstra-ami-builder/functions/cloudinit/src"
)

func main() {
	lambda.Start(cloudinit.HandleRequest)
}
