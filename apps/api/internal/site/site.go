// Package site はサイトそのものの値。機能をまたいで使うので、feature の外に置く（JUK-156）。
package site

// URL は本番の画面のオリジン。通知の本文のリンク・SEO の正規 URL・LINE の連携先に使う。
// 画面の src/shared/site.ts と同じ値。
const URL = "https://juken-map.com"
