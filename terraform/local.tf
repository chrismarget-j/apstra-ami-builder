locals {
  // artifacts go here until terraform sprouts a tmpfile capability
  tmp_dir = abspath("${path.root}/.terraform/tmp")

  // go binary name
  lambda_handler_name        = "lambda"

  // go binary location
  lambda_bin_file            = "${local.tmp_dir}/${local.lambda_handler_name}"

  // source folder within this project, used during build
  lambda_src_dir             = abspath("${path.module}/../functions/upload")

  // lambda service requires zip format files, here's where we put it
  lambda_zip_file            = "${local.tmp_dir}/${local.lambda_handler_name}.zip"
}
