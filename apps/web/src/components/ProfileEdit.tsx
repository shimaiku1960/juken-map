import { useState } from "react";
import { useSession } from "@/web/lib/auth-client";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { profileSchema, type ProfileInput } from "@/shared/validations/profile";
import { Button } from "@/web/components/ui/button";
import { Input } from "@/web/components/ui/input";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from "@/web/components/ui/form";
import { toast } from "sonner";
import { notifyDemoReadOnly } from "@/web/lib/demo-client";

const ProfileEdit = ({
  currentNickname,
  readOnly = false,
}: {
  currentNickname: string;
  readOnly?: boolean;
}) => {
  const { refetch } = useSession();
  const [isEditing, setIsEditing] = useState(false);

  const form = useForm<ProfileInput>({
    resolver: zodResolver(profileSchema),
    defaultValues: {
      nickname: currentNickname,
    },
  });

  const onSubmit = async (data: ProfileInput) => {
    const res = await fetch("/api/profile", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });

    if (res.ok) {
      toast.success("更新しました");
      setIsEditing(false);
      // Next.js では router.refresh() でサーバー再描画していた。SPA に相当物は無い。
      // authClient.getSession() は単発取得で useSession の購読ストアを更新しないため、
      // 表示が古いままになる（実際に踏んだ）。useSession の refetch を使うこと。
      await refetch();
    } else {
      const result = await res.json();
      toast.error(result.error || "更新に失敗しました");
    }
  };

  // 表示モード: 値 ＋ 右端に「編集」
  if (!isEditing) {
    return (
      <div className="flex items-center justify-between gap-2">
        <p className="text-lg">{currentNickname}</p>
        <Button
          type="button"
          variant="outline"
          size="lg"
          className="h-11"
          title={readOnly ? "デモアカウントは閲覧専用です" : undefined}
          onClick={() => {
            if (readOnly) {
              notifyDemoReadOnly();
              return;
            }
            form.reset({ nickname: currentNickname });
            setIsEditing(true);
          }}
        >
          編集
        </Button>
      </div>
    );
  }

  // 編集モード: その行が入力欄＋保存／キャンセルに変わる
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className="flex items-start gap-2"
      >
        <FormField
          control={form.control}
          name="nickname"
          render={({ field }) => (
            <FormItem className="flex-1">
              <FormControl>
                <Input autoFocus {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <Button
          type="submit"
          size="lg"
          className="h-11"
          disabled={form.formState.isSubmitting}
        >
          {form.formState.isSubmitting ? "保存中…" : "保存"}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="lg"
          className="h-11"
          onClick={() => setIsEditing(false)}
        >
          キャンセル
        </Button>
      </form>
    </Form>
  );
};

export default ProfileEdit;
