# コンソールへの日常のログインを IAM Identity Center にする（JUK-189）。
#
# 人のログインは、アクセスポータルから受け取る期限つきの権限で行う。root はパスワードと MFA を
# 残したまま、請求の設定など root にしかできない操作のときだけ使う。
#
# Identity Center 自体（東京の単一リージョン、暗号化は AWS 所有のキー）と Organizations は
# 2026-10-14 に画面で有効にした（有効にする API が無い）。ユーザーはメールアドレスを持つので
# 公開リポジトリには置かず、画面で作った。ここでは権限セットと、このアカウントへの割り当てだけを持つ。

variable "console_user_name" {
  description = "Identity Center で画面から作った、日常のログインに使うユーザーのユーザー名"
  type        = string
  default     = "Ikuro"
}

data "aws_ssoadmin_instances" "main" {}

locals {
  sso_instance_arn  = tolist(data.aws_ssoadmin_instances.main.arns)[0]
  identity_store_id = tolist(data.aws_ssoadmin_instances.main.identity_store_ids)[0]
}

data "aws_identitystore_user" "console" {
  identity_store_id = local.identity_store_id

  alternate_identifier {
    unique_attribute {
      attribute_path  = "UserName"
      attribute_value = var.console_user_name
    }
  }
}

# root で行っていた作業がそのままできるよう、まずは AdministratorAccess にする。
# 1回のログインで8時間（作業1日分）使える。既定の1時間だと、作業の途中で何度も切れる。
resource "aws_ssoadmin_permission_set" "admin" {
  name             = "AdministratorAccess"
  description      = "日常のコンソール操作（JUK-189）"
  instance_arn     = local.sso_instance_arn
  session_duration = "PT8H"
}

resource "aws_ssoadmin_managed_policy_attachment" "admin" {
  instance_arn       = local.sso_instance_arn
  permission_set_arn = aws_ssoadmin_permission_set.admin.arn
  managed_policy_arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}

resource "aws_ssoadmin_account_assignment" "console_admin" {
  instance_arn       = local.sso_instance_arn
  permission_set_arn = aws_ssoadmin_permission_set.admin.arn
  principal_type     = "USER"
  principal_id       = data.aws_identitystore_user.console.user_id
  target_type        = "AWS_ACCOUNT"
  target_id          = local.account_id

  # 権限セットにポリシーが付いてから割り当てる。逆だと、割り当て時に空の権限でロールが作られる。
  depends_on = [aws_ssoadmin_managed_policy_attachment.admin]
}
