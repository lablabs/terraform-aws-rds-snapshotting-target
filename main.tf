resource "aws_lambda_function" "this" {
  filename      = "${path.module}/lambda.zip"
  function_name = var.lambda_name
  role          = aws_iam_role.this.arn
  handler       = "main"

  timeout = "900"
  source_code_hash = filebase64sha256("${path.module}/lambda.zip")

  runtime = "go1.x"

  environment {
    variables = {
      REGION            = var.region
      KMS_KEY_ID        = module.kms_key.key_arn,
      RETENTION_DAYS    = var.db_snapshot_retention_days
    }
  }
}

resource "aws_iam_policy" "this" {
  name = var.lambda_name
  path = "/"

  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
     {
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": "arn:aws:logs:*:*:*",
      "Effect": "Allow"
     },
     {
      "Effect": "Allow",
      "Action": [
        "rds:CreateDBClusterSnapshot",
        "rds:DescribeDBClusterSnapshots",
        "rds:DescribeDBClusterSnapshotAttributes",
        "rds:ModifyDBClusterSnapshotAttribute",
        "rds:CopyDBClusterSnapshot",
        "rds:CopyDBClusterSnapshot",
        "rds:AddTagsToResource",
        "rds:DeleteDBClusterSnapshot",
        "rds:ListTagsForResource"
      ],
      "Resource": ["*"]
    },
    {
      "Effect": "Allow",
      "Action": [
        "kms:Encrypt",
        "kms:Decrypt",
        "kms:ReEncrypt*",
        "kms:CreateGrant",
        "kms:GenerateDataKey*",
        "kms:DescribeKey"
      ],
      "Resource": ["*"]
    }
  ]
}
EOF
}

resource "aws_iam_role_policy_attachment" "this" {
  role       = aws_iam_role.this.name
  policy_arn = aws_iam_policy.this.arn
}


resource "aws_sns_topic" "this" {
  name = var.lambda_name
  policy = data.aws_iam_policy_document.sns.json
}

resource "aws_iam_role" "this" {
  name = var.lambda_name

  assume_role_policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Action": "sts:AssumeRole",
      "Principal": {
        "Service": "lambda.amazonaws.com"
      },
      "Effect": "Allow",
      "Sid": ""
    }
  ]
}
EOF
}

resource "aws_lambda_permission" "this" {
  statement_id  = aws_lambda_function.this.function_name
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.this.function_name
  principal     = "sns.amazonaws.com"
  source_arn    = aws_sns_topic.this.arn
}

resource "aws_sns_topic_subscription" "this" {
  topic_arn = aws_sns_topic.this.arn
  protocol  = "lambda"
  endpoint  = aws_lambda_function.this.arn
}


data "aws_iam_policy_document" "sns" {

  statement {
    sid = "1"
    effect = "Allow"
    actions = [
      "SNS:GetTopicAttributes",
      "SNS:SetTopicAttributes",
      "SNS:AddPermission",
      "SNS:RemovePermission",
      "SNS:DeleteTopic",
      "SNS:Subscribe",
      "SNS:ListSubscriptionsByTopic",
      "SNS:Publish",
      "SNS:Receive",
    ]

    condition {
      test     = "StringEquals"
      variable = "AWS:SourceOwner"

      values = [data.aws_caller_identity.current.account_id]
    }

    principals {
      type        = "AWS"
      identifiers = ["*"]
    }

    resources = ["*"]
  }

  statement {
    sid = "2"
    effect = "Allow"
    actions = [
      "SNS:Publish",
    ]

    principals {
      type        = "AWS"
      identifiers = formatlist(
        "arn:aws:iam::%s:root",
        var.source_account_ids
      )
    }

    principals {
      identifiers = ["lambda.amazonaws.com"]
      type        = "Service"
    }

    resources = ["*"]
  }

}

module "kms_key" {
  source  = "cloudposse/kms-key/aws"
  version = "0.9.1"

  name                    = var.lambda_name
  description             = "KMS key for Aurora snapshots"
  deletion_window_in_days = 30
  enable_key_rotation     = true
  alias                   = "alias/${var.lambda_name}"
  policy                  = data.aws_iam_policy_document.kms.json
}


data "aws_caller_identity" "current" {}

data "aws_iam_policy_document" "kms" {
  statement {
    sid    = "AllowSnapshotCopy"
    effect = "Allow"
    actions = [
      "kms:CreateGrant",
      "kms:DescribeKey"
    ]

    resources = ["*"]


    principals {
      identifiers = ["lambda.amazonaws.com"]
      type        = "Service"
    }

  }

  statement {
    sid       = "Enable IAM User Permissions"
    effect    = "Allow"
    actions   = ["kms:*"]
    resources = ["*"]

    principals {
      identifiers = ["arn:aws:iam::${data.aws_caller_identity.current.account_id}:root"]
      type        = "AWS"
    }
  }
}
