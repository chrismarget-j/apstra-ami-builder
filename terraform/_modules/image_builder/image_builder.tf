resource "aws_imagebuilder_component" "update" {
  description = "update distribution package metadata"
  version     = "0.0.1"
  name        = "${local.project_name}-package-update"
  platform    = "Linux"
  data        = file("${path.module}/components/update.yml")
}

resource "aws_imagebuilder_component" "disable_upgrades" {
  description = "disable unattended upgrades"
  version     = "0.0.1"
  name        = "${local.project_name}-disable-upgrades"
  platform    = "Linux"
  data        = file("${path.module}/components/disable_unattended_upgrades.yml")
}

resource "aws_imagebuilder_component" "packages" {
  description = "install required packages"
  version     = "0.0.1"
  name        = "${local.project_name}-packages"
  platform    = "Linux"
  data        = file("${path.module}/components/packages.yml")
}

resource "aws_imagebuilder_component" "ami_builder" {
  description = "install AMI builder script"
  version     = "0.0.1"
  name        = "${local.project_name}-ami-builder"
  platform    = "Linux"
  data        = templatefile("${path.module}/components/ami_builder.yml", {
    SOURCE = "s3://${aws_s3_bucket.ours.bucket}/${aws_s3_object.ami_builder.key}"
#    DESTINATION = "/usr/local/scripts/${aws_s3_object.ami_builder.key}"
    DESTDIR = "/root"
    FILENAME = aws_s3_object.ami_builder.key
  })
}

resource "aws_imagebuilder_component" "aws_cli" {
  description = "install AWS CLI"
  version     = "0.0.1"
  name        = "${local.project_name}-aws-cli"
  platform    = "Linux"
  data        = file("${path.module}/components/aws_cli.yml")
}

resource "aws_imagebuilder_image_recipe" "ours" {
  name         = local.project_name
  version      = "0.0.1"
  parent_image = data.aws_ami.ubuntu_2204.id

  block_device_mapping {
    device_name = "/dev/sda1"

    ebs {
      delete_on_termination = true
      volume_size           = 8
      volume_type           = "gp3"
    }
  }

  component { component_arn = aws_imagebuilder_component.disable_upgrades.arn }
  component { component_arn = aws_imagebuilder_component.update.arn }
  component { component_arn = aws_imagebuilder_component.packages.arn }
  component { component_arn = aws_imagebuilder_component.aws_cli.arn }
  component { component_arn = aws_imagebuilder_component.ami_builder.arn }
}

resource "aws_imagebuilder_infrastructure_configuration" "ours" {
  name                          = local.project_name
  instance_profile_name         = aws_iam_instance_profile.image_builder.name
  instance_types                = ["t3a.micro"]
  terminate_instance_on_failure = true
}

resource "aws_imagebuilder_distribution_configuration" "ours" {
  name = local.project_name
  distribution {
    region = data.aws_region.ours.id
    ami_distribution_configuration {
      ami_tags = {
        Name = local.project_name
      }

    }
  }
}

resource "aws_imagebuilder_image_pipeline" "ours" {
  name                             = local.project_name
  infrastructure_configuration_arn = aws_imagebuilder_infrastructure_configuration.ours.arn
  image_recipe_arn                 = aws_imagebuilder_image_recipe.ours.arn
  distribution_configuration_arn = aws_imagebuilder_distribution_configuration.ours.arn
  image_tests_configuration {
    image_tests_enabled = false
  }
}
