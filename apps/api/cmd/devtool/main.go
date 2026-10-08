// 開発でしか使わない道具（seed・E2E・負荷試験の下ごしらえ）。本番のイメージには入れない（Dockerfile は cmd/api だけを作る）。
// 中身は internal/devtool にある（JUK-143）。手元では scripts/go-devtool.sh から呼ぶ。
package main

import (
	"os"

	"github.com/shimaiku1960/juken-map/apps/api/internal/devtool"
)

func main() {
	os.Exit(devtool.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
