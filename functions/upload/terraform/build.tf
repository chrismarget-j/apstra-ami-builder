// This data provider zips up the Go source directory only for the purpose of
// calculating its checksum. Unfortunately, the checksum is calculated after the
// file is written, so we're required to write output to a real file.
data "archive_file" "watch_src_dir" {
  type        = "zip"
  source_dir  = local.src_dir
  output_path = "${local.scratchpad_dir}/watch_src_dir.garbage.zip"
}

// Build the project whenever the archive_file.watch_src_dir detects changes
resource "null_resource" "build_project" {
  triggers = {
    doit = data.archive_file.watch_src_dir.output_md5
  }
  provisioner "local-exec" {
    working_dir = local.src_dir
    command     = "GOOS=linux GOARCH=amd64 go build -o ${local.bin_file} ${local.main}"
  }
}

// AWS requires lambdas be delivered in .zip format
data "archive_file" "zipped_for_lambda" {
  type        = "zip"
  source_file = local.bin_file
  output_path = local.zip_file
  depends_on  = [null_resource.build_project]
}
