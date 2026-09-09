#!/usr/bin/env node
// Next.js のコンポーネントを apps/web（Vite + React Router）へ移すときの機械置換。
// 手作業だと 17 ファイルの next/link と 11 ファイルの next/navigation を取りこぼすため、
// 変換できるものだけを機械的に処理し、判断が要るものは目印を残して報告する。
import { readFileSync, writeFileSync } from "node:fs";

const files = process.argv.slice(2);
const report = [];

for (const file of files) {
  let s = readFileSync(file, "utf8");
  const before = s;
  const notes = [];

  // "use client" は SPA では全部クライアントなので不要
  s = s.replace(/^["']use client["'];?\n+/m, "");

  // next/link → react-router の Link。href は to になる。
  if (/from "next\/link"/.test(s)) {
    s = s.replace(/import Link from "next\/link";?\n/, 'import { Link } from "react-router";\n');
    s = s.replace(/<Link([^>]*?)\shref=/g, "<Link$1 to=");
  }

  // next/navigation
  if (/from "next\/navigation"/.test(s)) {
    const imported = (s.match(/import \{([^}]*)\} from "next\/navigation";?/) || [, ""])[1]
      .split(",").map((x) => x.trim()).filter(Boolean);
    const routerImports = new Set();
    for (const name of imported) {
      if (name === "useRouter") routerImports.add("useNavigate");
      else if (name === "usePathname") routerImports.add("useLocation");
      else if (name === "useSearchParams") routerImports.add("useSearchParams");
      else if (name === "useParams") routerImports.add("useParams");
      else notes.push(`next/navigation の ${name} は手当てが必要（サーバー専用API）`);
    }
    s = s.replace(/import \{[^}]*\} from "next\/navigation";?\n/,
      routerImports.size ? `import { ${[...routerImports].sort().join(", ")} } from "react-router";\n` : "");

    s = s.replace(/const router = useRouter\(\);?/g, "const navigate = useNavigate();");
    s = s.replace(/const pathname = usePathname\(\);?/g, "const pathname = useLocation().pathname;");
    s = s.replace(/router\.push\(([^)]*)\)/g, "navigate($1)");
    s = s.replace(/router\.replace\(([^)]*)\)/g, "navigate($1, { replace: true })");
    if (/router\.refresh\(\)/.test(s)) {
      notes.push("router.refresh() は SPA に相当物が無い。TanStack Query の invalidate へ置き換えること");
    }
  }

  // next/image → 素の img。fill / priority など Next 固有の props は残らないよう目印を出す。
  if (/from "next\/image"/.test(s)) {
    notes.push("next/image を使用。<img> への置換とサイズ指定の見直しが必要");
  }

  // import パスをこのパッケージの別名へ
  s = s.replace(/(["'])@\/frontend\/components\//g, "$1@/web/components/");
  s = s.replace(/(["'])@\/frontend\/hooks\//g, "$1@/web/hooks/");
  s = s.replace(/(["'])@\/frontend\/lib\//g, "$1@/web/lib/");

  if (s !== before) writeFileSync(file, s);
  report.push({ file, changed: s !== before, notes });
}

for (const r of report) {
  const mark = r.changed ? "変換" : "無変更";
  console.log(`${mark}  ${r.file}`);
  r.notes.forEach((n) => console.log(`        ⚠️  ${n}`));
}
