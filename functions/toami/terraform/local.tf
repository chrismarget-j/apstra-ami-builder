locals {
  // scratchpad for this function within var.scratchpad_dir
  scratchpad_dir = abspath("${path.root}/.terraform/tmp/${basename(local.src_dir)}")

  // source folder within this project, used during build
  src_dir = abspath("${path.module}/..")

  // go binary name
  handler_name = "lambda"

  // go binary location
  bin_file = "${local.scratchpad_dir}/${local.handler_name}"

  // lambda service requires zip format files, here's where we put it
  zip_file = "${local.bin_file}.zip"

  // file within source tree containing 'func main()'
  main = "lambda/main.go"

  // prefix/suffix used to trigger lambda and in lambda IAM policy to allow object fetch
  bucket_key_prefix = ""
  bucket_key_suffix = ".vmdk"
}
