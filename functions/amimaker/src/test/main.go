package main

import (
	"context"
	"encoding/json"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"log"
)

func main() {
	ctx := context.Background()

	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatal(err)
	}

	client := ec2.NewFromConfig(awsCfg)

	params := &ec2.RegisterImageInput{
		Name:         aws.String("name goes here"),
		Architecture: "x86_64",
		BlockDeviceMappings: []types.BlockDeviceMapping{{
			DeviceName: aws.String("/dev/sda1"),
			Ebs: &types.EbsBlockDevice{
				DeleteOnTermination: aws.Bool(true),
				SnapshotId:          aws.String("snap-010d9f56412e3c26c"),
				VolumeType:          "gp2",
			},
		}},
		Description:        aws.String("amimaker description goes here"),
		EnaSupport:         aws.Bool(true),
		ImdsSupport:        "v2.0",
		RootDeviceName:     aws.String("/dev/sda1"),
		SriovNetSupport:    nil,
		VirtualizationType: aws.String("hvm"),
	}

	rio, err := client.RegisterImage(ctx, params)
	dump, err := json.Marshal(rio)
	log.Println(string(dump))
	if err != nil {
		log.Fatal(err)
	}
	log.Println("no error")

}
