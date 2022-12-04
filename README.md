# apstra-ami-builder

This repository faciliates running Juniper Apstra in AWS by doing the following:

1. Download Apstra software images (VMware/OVA format) from Juniper's CDN to your AWS account.
1. Extract the included disk image (VMDK) to an S3 bucket.
1. ~~Convert the VMDK to an EBS snapshot.~~
1. ~~Link the EBS snapshot to an AMI.~~
1. ~~Boot the AMI as an EC2 instance, install [cloud-init](https://cloud-init.io).~~
1. ~~Shut down the EC2 instance, convert to a new AMI.~~
1. ~~Clean up the intermediate EBS snapshot, AMI and EC2 instances.~~
1. ~~Distribute the new AMI to prescribed AWS regions.~~

#### Prerequisites
1. An AWS account.
1. Credentials for the AWS account in your environment/config, with sufficient permissions to create various AWS resources. todo: enumerate these
1. Software on your deployment system:
    - Git (to clone this repo)
    - Terraform (to deploy the AWS resources)
    - Go (to build the labmda functions)
    - AWS CLI (to kick off the process after deployment)
    - jq (used in the deployment process script to process json)
    
#### Usage

###### Step 1 - deploy aws resources

1. Install prerequisite software.
1. Clone this repository to your local system.  
  ```shell
  git clone https://github.com/chrismarget-j/apstra-ami-builder
  cd apstra-ami-builder
  ```
1. Deploy resources to AWS. These resources handle the logic for accomplishing
the steps outlined above, but don't have direct access to the Apstra software.
We'll give it tokenized links to your preferred Apstra version next.
  ```shell
  terraform -chdir=terraform apply
  ```
1. Run the deploy script and follow the prompts. It will ask for a tokenized 
download link from the Juniper software download page.
  ```shell
  ./deploy_from_juniper_cdn.sh
  ```
