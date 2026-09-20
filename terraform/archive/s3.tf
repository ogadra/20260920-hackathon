# trivy:ignore:AVD-AWS-0089 -- Access logging on an archive bucket would itself generate logs to archive
resource "aws_s3_bucket" "logs_apne1" {
  # checkov:skip=CKV_AWS_18:Access logging on an archive bucket would itself generate logs to archive
  # checkov:skip=CKV2_AWS_62:Event notifications are not required for archive storage
  # checkov:skip=CKV_AWS_144:Cross-region replication is not required for archive storage
  # checkov:skip=CKV_AWS_145:SSE-S3 is sufficient for logs already retained in CloudWatch Logs
  provider = aws.apne1

  bucket = format("bunshin-logs-%s-ap-northeast-1", data.aws_caller_identity.current.account_id)

  object_lock_enabled = true

  tags = merge(local.common_tags, {
    Name    = format("bunshin-logs-%s-ap-northeast-1", data.aws_caller_identity.current.account_id)
    Service = "logs"
  })

  lifecycle {
    prevent_destroy = true
  }
}

# trivy:ignore:AVD-AWS-0089 -- Access logging on an archive bucket would itself generate logs to archive
resource "aws_s3_bucket" "logs_apne3" {
  # checkov:skip=CKV_AWS_18:Access logging on an archive bucket would itself generate logs to archive
  # checkov:skip=CKV2_AWS_62:Event notifications are not required for archive storage
  # checkov:skip=CKV_AWS_144:Cross-region replication is not required for archive storage
  # checkov:skip=CKV_AWS_145:SSE-S3 is sufficient for logs already retained in CloudWatch Logs
  provider = aws.apne3

  bucket = format("bunshin-logs-%s-ap-northeast-3", data.aws_caller_identity.current.account_id)

  object_lock_enabled = true

  tags = merge(local.common_tags, {
    Name    = format("bunshin-logs-%s-ap-northeast-3", data.aws_caller_identity.current.account_id)
    Service = "logs"
  })

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_public_access_block" "logs_apne1" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne1

  bucket = aws_s3_bucket.logs_apne1.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_public_access_block" "logs_apne3" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne3

  bucket = aws_s3_bucket.logs_apne3.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_versioning" "logs_apne1" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne1

  bucket = aws_s3_bucket.logs_apne1.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_versioning" "logs_apne3" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne3

  bucket = aws_s3_bucket.logs_apne3.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_object_lock_configuration" "logs_apne1" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne1

  bucket = aws_s3_bucket.logs_apne1.id

  rule {
    default_retention {
      mode  = "GOVERNANCE"
      years = 1
    }
  }

  depends_on = [aws_s3_bucket_versioning.logs_apne1]
}

resource "aws_s3_bucket_object_lock_configuration" "logs_apne3" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne3

  bucket = aws_s3_bucket.logs_apne3.id

  rule {
    default_retention {
      mode  = "GOVERNANCE"
      years = 1
    }
  }

  depends_on = [aws_s3_bucket_versioning.logs_apne3]
}

# trivy:ignore:AVD-AWS-0132 -- SSE-S3 is sufficient for logs already retained in CloudWatch Logs
resource "aws_s3_bucket_server_side_encryption_configuration" "logs_apne1" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne1

  bucket = aws_s3_bucket.logs_apne1.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }

    bucket_key_enabled = true
  }
}

# trivy:ignore:AVD-AWS-0132 -- SSE-S3 is sufficient for logs already retained in CloudWatch Logs
resource "aws_s3_bucket_server_side_encryption_configuration" "logs_apne3" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne3

  bucket = aws_s3_bucket.logs_apne3.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }

    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "logs_apne1" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne1

  bucket = aws_s3_bucket.logs_apne1.id

  rule {
    id     = "archive-to-glacier-ir"
    status = "Enabled"

    filter {}

    transition {
      days          = 7
      storage_class = "GLACIER_IR"
    }
  }

  rule {
    id     = "abort-incomplete-multipart-upload"
    status = "Enabled"

    filter {}

    abort_incomplete_multipart_upload {
      days_after_initiation = 1
    }
  }

  depends_on = [aws_s3_bucket_versioning.logs_apne1]
}

resource "aws_s3_bucket_lifecycle_configuration" "logs_apne3" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne3

  bucket = aws_s3_bucket.logs_apne3.id

  rule {
    id     = "archive-to-glacier-ir"
    status = "Enabled"

    filter {}

    transition {
      days          = 7
      storage_class = "GLACIER_IR"
    }
  }

  rule {
    id     = "abort-incomplete-multipart-upload"
    status = "Enabled"

    filter {}

    abort_incomplete_multipart_upload {
      days_after_initiation = 1
    }
  }

  depends_on = [aws_s3_bucket_versioning.logs_apne3]
}

data "aws_iam_policy_document" "logs_apne1" {
  provider = aws.apne1

  statement {
    effect = "Deny"

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    actions = ["s3:*"]
    resources = [
      aws_s3_bucket.logs_apne1.arn,
      "${aws_s3_bucket.logs_apne1.arn}/*",
    ]

    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }

    condition {
      test     = "Bool"
      variable = "aws:PrincipalIsAWSService"
      values   = ["false"]
    }
  }

  statement {
    principals {
      type        = "Service"
      identifiers = ["logs.ap-northeast-1.amazonaws.com"]
    }

    actions   = ["s3:GetBucketAcl"]
    resources = [aws_s3_bucket.logs_apne1.arn]

    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
  }

  statement {
    principals {
      type        = "Service"
      identifiers = ["logs.ap-northeast-1.amazonaws.com"]
    }

    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.logs_apne1.arn}/*"]

    condition {
      test     = "StringEquals"
      variable = "s3:x-amz-acl"
      values   = ["bucket-owner-full-control"]
    }

    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
  }
}

data "aws_iam_policy_document" "logs_apne3" {
  provider = aws.apne3

  statement {
    effect = "Deny"

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    actions = ["s3:*"]
    resources = [
      aws_s3_bucket.logs_apne3.arn,
      "${aws_s3_bucket.logs_apne3.arn}/*",
    ]

    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }

    condition {
      test     = "Bool"
      variable = "aws:PrincipalIsAWSService"
      values   = ["false"]
    }
  }

  statement {
    principals {
      type        = "Service"
      identifiers = ["logs.ap-northeast-3.amazonaws.com"]
    }

    actions   = ["s3:GetBucketAcl"]
    resources = [aws_s3_bucket.logs_apne3.arn]

    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
  }

  statement {
    principals {
      type        = "Service"
      identifiers = ["logs.ap-northeast-3.amazonaws.com"]
    }

    actions   = ["s3:PutObject"]
    resources = ["${aws_s3_bucket.logs_apne3.arn}/*"]

    condition {
      test     = "StringEquals"
      variable = "s3:x-amz-acl"
      values   = ["bucket-owner-full-control"]
    }

    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
  }
}

resource "aws_s3_bucket_policy" "logs_apne1" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne1

  bucket = aws_s3_bucket.logs_apne1.id
  policy = data.aws_iam_policy_document.logs_apne1.json
}

resource "aws_s3_bucket_policy" "logs_apne3" {
  # checkov:skip=CKV_BUNSHIN_1:Resource does not support tags
  provider = aws.apne3

  bucket = aws_s3_bucket.logs_apne3.id
  policy = data.aws_iam_policy_document.logs_apne3.json
}
