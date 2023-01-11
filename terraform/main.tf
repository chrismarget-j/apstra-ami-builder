module "image_builder" {
  source = "./_modules/image_builder"
}

output "apstra_ami_builder_role_name" {
  value = module.image_builder.apstra_ami_builder_role_name
}