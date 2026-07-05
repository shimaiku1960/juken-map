resource "aws_instance" "app" {
  ami                    = "ami-0126975fb247bf2e7"
  instance_type          = "t3.micro"
  key_name               = "juken-compass-key"
  subnet_id              = "subnet-0d4499710ef9ef629"
  vpc_security_group_ids = [aws_security_group.launch_wizard_2.id]

  root_block_device {
    volume_size = 30
  }

  iam_instance_profile = aws_iam_instance_profile.ec2_ecr.name

  tags = {
    Name = "juken-compass"
  }
}