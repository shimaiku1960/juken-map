#!/usr/bin/env bash
# 負荷をかける EC2 の初回起動で動く（terraform/loadtest/ec2.tf の user_data に埋め込まれる）。
# 手前で REPO_REF・PARAM_NAME・RESULTS_BUCKET・AUTO_TERMINATE_MINUTES が export されている。
# 終わると /var/lib/juken-map-loadtest-ready ができる。ログは /var/log/juken-map-loadtest-setup.log。
set -euo pipefail
exec > >(tee -a /var/log/juken-map-loadtest-setup.log) 2>&1

# 壊し忘れの保険。止まると terminate される（instance_initiated_shutdown_behavior）。
shutdown -h "+${AUTO_TERMINATE_MINUTES}" "juken-map-loadtest: auto terminate"

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates curl git gnupg jq mysql-client unzip

# k6（公式の apt リポジトリ）
gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg \
  --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" \
  > /etc/apt/sources.list.d/k6.list
apt-get update
apt-get install -y k6

# Node 24（アプリと同じ）と、package.json の packageManager に固定した pnpm
curl -fsSL https://deb.nodesource.com/setup_24.x | bash -
apt-get install -y nodejs
corepack enable

# AWS CLI v2（S3 へ結果を上げる）
curl -fsSL https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip -o /tmp/awscliv2.zip
unzip -q /tmp/awscliv2.zip -d /tmp
/tmp/aws/install

# 本番で動いているのと同じコミットを置く（マイグレーションと seed をアプリと揃えるため）。
sudo -u ubuntu -H git clone https://github.com/shimaiku1960/juken-map.git /home/ubuntu/juken-map
sudo -u ubuntu -H git -C /home/ubuntu/juken-map checkout --detach "$REPO_REF"

# 試験用の DATABASE_URL と BETTER_AUTH_SECRET（本番の値ではない）。
umask 077
aws ssm get-parameter --region ap-northeast-1 --name "$PARAM_NAME" --with-decryption \
  --query Parameter.Value --output text > /home/ubuntu/juken-map/.env
chown ubuntu:ubuntu /home/ubuntu/juken-map/.env
umask 022

sudo -u ubuntu -H bash -c 'cd ~/juken-map && COREPACK_ENABLE_DOWNLOAD_PROMPT=0 pnpm install --frozen-lockfile'

cat > /etc/profile.d/juken-map-loadtest.sh <<EOF
export LOADTEST_ENV=aws
export RESULTS_BUCKET='$RESULTS_BUCKET'
EOF

touch /var/lib/juken-map-loadtest-ready
echo "juken-map-loadtest: setup finished"
