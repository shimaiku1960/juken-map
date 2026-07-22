"use client";

import { useState } from "react";
import Link from "next/link";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import PrefectureModal from "./PrefectureModal";
import FacultyTagModal from "./FacultyTagModal";

type University = {
  id: number;
  name: string;
  prefecture: string;
  type: string;
  facultyCount: number;
  tagNames: string[];
};

type Props = {
  universities: University[];
};

const TYPES = ["国立", "公立", "私立"];

export default function UniversitySearch({ universities }: Props) {
  const [name, setName] = useState("");
  const [selectedTypes, setSelectedTypes] = useState<Set<string>>(new Set());
  const [selectedPrefs, setSelectedPrefs] = useState<Set<string>>(new Set());
  const [selectedTags, setSelectedTags] = useState<Set<string>>(new Set());

  type Conditions = {
    name: string;
    types: Set<string>;
    prefs: Set<string>;
    tags: Set<string>;
  };
  const [applied, setApplied] = useState<Conditions | null>(null);

  const toggleType = (type: string) => {
    setSelectedTypes((prev) => {
      const next = new Set(prev);
      if (next.has(type)) next.delete(type);
      else next.add(type);
      return next;
    });
  };

  const reset = () => {
    setName("");
    setSelectedTypes(new Set());
    setSelectedPrefs(new Set());
    setSelectedTags(new Set());
    setApplied(null);
  };

  // Set が空の条件はスキップ＝絞り込まない
  const matches = (u: University, c: Conditions) => {
    if (c.name.trim() !== "" && !u.name.includes(c.name.trim())) return false;
    if (c.types.size > 0 && !c.types.has(u.type)) return false;
    if (c.prefs.size > 0 && !c.prefs.has(u.prefecture)) return false;
    if (c.tags.size > 0 && !u.tagNames.some((t) => c.tags.has(t))) return false;
    return true;
  };

  const draft: Conditions = {
    name,
    types: selectedTypes,
    prefs: selectedPrefs,
    tags: selectedTags,
  };

  const previewCount = universities.filter((u) => matches(u, draft)).length;
  const results = applied
    ? universities.filter((u) => matches(u, applied))
    : null;

  return (
    <div>
      <div className="border rounded-lg p-4 space-y-4 mb-6">
        <Input
          placeholder="大学名で探す"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />

        {/* フィルタチップ行：設置区分はインライン、それ以外はモーダル */}
        <div className="flex flex-wrap gap-2">
          {TYPES.map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => toggleType(t)}
              className={`rounded-md border px-4 py-2 text-sm transition-colors ${
                selectedTypes.has(t)
                  ? "border-primary bg-primary text-primary-foreground"
                  : "bg-card text-foreground hover:bg-muted/50"
              }`}
            >
              {t}
            </button>
          ))}

          <PrefectureModal value={selectedPrefs} onChange={setSelectedPrefs}>
            <button
              type="button"
              className="rounded-md border px-4 py-2 text-sm hover:bg-muted/50 flex items-center gap-2"
            >
              都道府県を選ぶ
              {selectedPrefs.size > 0 && (
                <Badge variant="secondary">{selectedPrefs.size}</Badge>
              )}
            </button>
          </PrefectureModal>

          <FacultyTagModal value={selectedTags} onChange={setSelectedTags}>
            <button
              type="button"
              className="rounded-md border px-4 py-2 text-sm hover:bg-muted/50 flex items-center gap-2"
            >
              学部系統を選ぶ
              {selectedTags.size > 0 && (
                <Badge variant="secondary">{selectedTags.size}</Badge>
              )}
            </button>
          </FacultyTagModal>

          <button
            type="button"
            disabled
            title="準備中（偏差値データは後日追加予定）"
            className="rounded-md border px-4 py-2 text-sm text-muted-foreground cursor-not-allowed"
          >
            偏差値を選ぶ（準備中）
          </button>
        </div>

        <div className="flex justify-between items-center pt-2 border-t">
          <p className="text-sm">
            対象 <span className="font-bold text-lg">{previewCount}</span> 校
          </p>
          <div className="flex items-center gap-4">
            <button
              onClick={reset}
              className="text-sm text-muted-foreground hover:underline"
            >
              条件をリセット
            </button>
            <Button onClick={() => setApplied(draft)}>検索</Button>
          </div>
        </div>
      </div>

      {results === null ? (
        <p className="text-muted-foreground text-sm">
          条件を選んで「検索」を押してください。
        </p>
      ) : results.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          条件に合う大学が見つかりませんでした。
        </p>
      ) : (
        <ul className="space-y-2">
          {results.map((u) => (
            <li key={u.id}>
              <Link
                href={`/explore/${u.id}`}
                className="border rounded px-4 py-3 flex justify-between items-center hover:bg-muted/50"
              >
                <div>
                  <p className="font-medium">{u.name}</p>
                  <p className="text-sm text-muted-foreground">
                    {u.prefecture} / {u.type}
                  </p>
                </div>
                <span className="text-sm text-muted-foreground">
                  {u.facultyCount > 0
                    ? `学部 ${u.facultyCount}`
                    : "学部情報準備中"}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
