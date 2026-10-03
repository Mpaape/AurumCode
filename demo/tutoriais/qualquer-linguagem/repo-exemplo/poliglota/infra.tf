resource "aws_s3_bucket" "logs" {
  bucket = "demo-logs"
  acl    = "private"
}
