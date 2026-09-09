import type { ReactNode } from "react";
import Link from "next/link";

type LegalSection = {
  title: string;
  content: ReactNode;
};

type LegalDocumentProps = {
  title: string;
  description: string;
  effectiveDate: string;
  sections: LegalSection[];
};

export default function LegalDocument({
  title,
  description,
  effectiveDate,
  sections,
}: LegalDocumentProps) {
  return (
    <main className="mx-auto w-full max-w-3xl px-4 py-10 sm:px-8 sm:py-14">
      <header className="border-b pb-8">
        <p className="text-sm font-medium text-primary">受験マップ</p>
        <h1 className="mt-2 text-3xl font-bold tracking-tight sm:text-4xl">
          {title}
        </h1>
        <p className="mt-4 max-w-2xl leading-7 text-muted-foreground">
          {description}
        </p>
        <p className="mt-4 text-sm text-muted-foreground">
          制定日：{effectiveDate}
        </p>
      </header>

      <div className="space-y-10 py-10">
        {sections.map((section, index) => (
          <section key={section.title} aria-labelledby={`section-${index + 1}`}>
            <h2
              id={`section-${index + 1}`}
              className="text-xl font-semibold tracking-tight"
            >
              {index + 1}. {section.title}
            </h2>
            <div className="mt-4 space-y-3 leading-7 text-muted-foreground [&_a]:font-medium [&_a]:text-primary [&_a]:underline [&_a]:underline-offset-4 [&_li]:pl-1 [&_ol]:ml-5 [&_ol]:list-decimal [&_ol]:space-y-2 [&_strong]:font-semibold [&_strong]:text-foreground [&_ul]:ml-5 [&_ul]:list-disc [&_ul]:space-y-2">
              {section.content}
            </div>
          </section>
        ))}
      </div>

      <nav
        aria-label="規約とポリシー"
        className="flex flex-wrap gap-x-6 gap-y-3 border-t pt-8 text-sm"
      >
        <Link href="/terms" className="text-primary hover:underline">
          利用規約
        </Link>
        <Link href="/privacy" className="text-primary hover:underline">
          プライバシーポリシー
        </Link>
        <Link href="/" className="text-muted-foreground hover:text-foreground">
          トップへ戻る
        </Link>
      </nav>
    </main>
  );
}
