variable "source_ami_id" {
  description = "本番 EC2 から scripts/loadtest-aws/create-source-ami.sh で作った AMI。本番の秘密情報を含むので試験後に消す"
  type        = string

  validation {
    condition     = can(regex("^ami-[0-9a-f]+$", var.source_ami_id))
    error_message = "ami- で始まる AMI の ID を渡してください。"
  }
}

variable "repo_ref" {
  description = "負荷をかける EC2 に置くコミット。本番で動いているイメージのタグ（＝コミット）と揃える"
  type        = string

  validation {
    condition     = can(regex("^[0-9a-f]{40}$", var.repo_ref))
    error_message = "40桁のコミットハッシュを渡してください。"
  }
}

variable "primary_az" {
  description = "複製 EC2・負荷をかける EC2・RDS を置く AZ（同じ AZ に揃えて、AZ をまたぐ遅れを混ぜない）"
  type        = string
  default     = "ap-northeast-1a"
}

variable "secondary_az" {
  description = "RDS のサブネットグループが2つの AZ を要求するためだけに使う"
  type        = string
  default     = "ap-northeast-1c"
}

variable "driver_instance_type" {
  description = "負荷をかける EC2。k6 の CPU が張り付いたら c7i.xlarge に上げる"
  type        = string
  default     = "c7i.large"
}

variable "auto_terminate_minutes" {
  description = "壊し忘れの保険。EC2 は起動からこの時間で自分を止め、止まると削除される（RDS には効かない）"
  type        = number
  default     = 720
}
