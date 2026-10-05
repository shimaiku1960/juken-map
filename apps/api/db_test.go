package main

import "testing"

func TestDBConfigUsesTLSOnlyForRDS(t *testing.T) {
	rds, err := dbConfig("mysql://juken_app:pw@juken-map-db.abc123.ap-northeast-1.rds.amazonaws.com:3306/juken_map")
	if err != nil {
		t.Fatal(err)
	}
	if rds.TLS == nil {
		t.Fatal("RDS なのに TLS が付いていない")
	}
	// ServerName が空だと、証明書のホスト名を確かめられない。
	if rds.TLS.ServerName != "juken-map-db.abc123.ap-northeast-1.rds.amazonaws.com" {
		t.Errorf("ServerName = %q", rds.TLS.ServerName)
	}
	if rds.TLS.InsecureSkipVerify {
		t.Error("証明書の確認を飛ばしている")
	}

	local, err := dbConfig("mysql://root:pw@127.0.0.1:3306/juken_map")
	if err != nil {
		t.Fatal(err)
	}
	if local.TLS != nil {
		t.Error("手元の MySQL は証明書を持たないので、TLS を付けない")
	}
}

func TestRDSBundleHasTokyoRoots(t *testing.T) {
	// ファイルを取り違えて空や別物になると、本番の起動時に初めて繋がらないと分かる。先に落とす。
	cfg, err := dbConfig("mysql://u:p@x.ap-northeast-1.rds.amazonaws.com/db")
	if err != nil {
		t.Fatal(err)
	}
	// Subjects は非推奨だが、数を数えるだけなら使える（信頼リストの中身を見る手段がほかに無い）。
	if n := len(cfg.TLS.RootCAs.Subjects()); n != 3 {
		t.Errorf("ルート証明書が %d 本（RSA2048・RSA4096・ECC384 の3本のはず）", n)
	}
}
