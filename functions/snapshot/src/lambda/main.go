package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/chrismarget-j/apstra-ami-builder/functions/snapshot"
)

func main() {
	lambda.Start(snapshot.HandleRequest)
}
