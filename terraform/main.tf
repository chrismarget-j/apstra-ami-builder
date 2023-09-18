module "image_builder" {
  source = "./_modules/image_builder"
}

module "apstra_testbed_role" {
  source = "./_modules/testbed_role"
}

output "apstra_ami_builder_role_name" {
  value = module.image_builder.apstra_ami_builder_role_name
}