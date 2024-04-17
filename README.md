# apstra-ami-builder

This repository faciliates running Juniper Apstra in AWS.

### Prerequisites
1. An AWS account.
1. Credentials for the AWS account in your environment/config, with sufficient permissions to create the required AWS
   resources.
1. Software on your deployment system:
    - Git (to clone this repo)
    - Terraform (to deploy the AWS resources)
    - AWS CLI (to kick off the process after deployment)
    - jq (used in the deployment process script to process json)
    
#### Usage

###### Step 1 - deploy AWS resources

1. Clone this repository to your local system.  
   ```shell
   git clone https://github.com/chrismarget-j/apstra-ami-builder
   cd apstra-ami-builder
   ```
1. Deploy resources to AWS. Broadly speaking, those are:
   - AWS ImageBuilder components, recipe, pipeline, etc... capable of building an AMI appropriate for processing a new
     Apstra release into an AWS AMI.
   - AWS IAM policies, etc... required to run ImageBuilder.
   - AWS IAM policies, etc... required by the Imagebuilder output to process an Apstra release.
   - AWS S3 bucket and object which house a script containing the Apstra Release -> AWS AMI conversion process.
   ```shell
   terraform -chdir=terraform init
   terraform -chdir=terraform apply
   ```
1. Run the ImageBuilder Pipeline. This step could be performed by Terraform, but it takes a while. I prefer running it
   via the web UI. From the AWS ImageBuilder console select the `apstra-ami-builder` pipeline and then Actions->Run
   Pipeline. Running the pipeline will create a new AMI in the account. Eventually.
1. Run the deploy script from the top level directory of this repository. It will prompt for an Apstra download URL,
   and then launch the `apstra-ami-builder` AMI as an EC2 instance. That instance will fetch the Apstra release from
   the supplied URL, process it into a cloud-init-enabled AMI in the account, and then shut itself down.
   ```shell
   ./deploy_from_juniper_cdn.sh
   ```

