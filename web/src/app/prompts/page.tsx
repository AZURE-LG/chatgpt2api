"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Copy, Globe2, LoaderCircle, Lock, Pencil, Plus, Search, Send, Trash2, X } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";

import { PageHeader } from "@/components/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { copyTextToClipboard } from "@/lib/clipboard";
import {
  copyPrompt,
  deletePrompt,
  fetchPrompts,
  savePrompt,
  type PromptLibraryItem,
  type PromptVisibility,
} from "@/lib/image-workspace-api";
import { useAuthGuard } from "@/lib/use-auth-guard";
import { hasAPIPermission, type StoredAuthSession } from "@/store/auth";

const PENDING_PROMPT_STORAGE_KEY = "chatgpt2api:pending_prompt";
const ALL_FILTER_VALUE = "__all";

type PromptScope = "visible" | "mine" | "public" | "all";

type PromptFormState = {
  title: string;
  body: string;
  tags: string;
  category: string;
  note: string;
  model: string;
  use_case: string;
  visibility: PromptVisibility;
};

const EMPTY_PROMPT_FORM: PromptFormState = {
  title: "",
  body: "",
  tags: "",
  category: "",
  note: "",
  model: "",
  use_case: "",
  visibility: "private",
};

function parsePromptTags(value: string) {
  return Array.from(
    new Set(
      value
        .split(/[,，\n]/)
        .map((item) => item.trim())
        .filter(Boolean),
    ),
  );
}

function stripImagePromptTitlePrefix(value: string) {
  return value.trim().replace(/^图片提示词(?:[:：]\s*)?/, "").trim();
}

function compactPromptTitle(value: string, maxLength = 34) {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (!normalized) {
    return "";
  }
  return normalized.length > maxLength ? `${normalized.slice(0, maxLength)}...` : normalized;
}

function promptFormFromItem(item: PromptLibraryItem): PromptFormState {
  const title =
    stripImagePromptTitlePrefix(item.title) ||
    (item.title.trim().startsWith("图片提示词") ? compactPromptTitle(item.body) || "未命名提示词" : item.title);
  return {
    title,
    body: item.body,
    tags: item.tags.join(", "),
    category: item.category,
    note: item.note,
    model: item.model,
    use_case: item.use_case,
    visibility: item.visibility,
  };
}

function canMutatePrompt(item: PromptLibraryItem, session: StoredAuthSession) {
  return item.owner_id === session.subjectId || (session.role === "admin" && item.visibility === "public");
}

function isGeneratedImageFileTitle(value: string) {
  const normalized = value.trim().replace(/\.[a-z0-9]+$/i, "");
  return /^\d{8,}_?[a-f0-9]{16,}$/i.test(normalized) || /^[a-f0-9]{24,}$/i.test(normalized);
}

function promptDisplayTitle(item: PromptLibraryItem) {
  const bodyTitle = item.source_image_path ? compactPromptTitle(item.body) : "";
  if (item.source_image_path && isGeneratedImageFileTitle(item.title)) {
    if (bodyTitle) {
      return bodyTitle;
    }
  }
  return stripImagePromptTitlePrefix(item.title) || bodyTitle || "未命名提示词";
}

function shouldShowOriginalPromptTitle(item: PromptLibraryItem, displayTitle: string) {
  if (item.title.trim().startsWith("图片提示词")) {
    return false;
  }
  if (stripImagePromptTitlePrefix(item.title) === displayTitle) {
    return false;
  }
  return displayTitle !== item.title;
}

function promptVisibilityLabel(visibility: PromptVisibility) {
  return visibility === "public" ? "公开" : "私有";
}

function promptVisibilityPillClass(visibility: PromptVisibility) {
  return visibility === "public"
    ? "border-[#bfdbfe] bg-[#e8f2ff] text-[#1456f0] dark:border-sky-500/40 dark:bg-sky-500/18 dark:text-sky-200"
    : "border-slate-700 bg-[#181e25] text-white dark:border-white/18 dark:bg-white/12 dark:text-white";
}

function uniquePromptValues(items: PromptLibraryItem[], selector: (item: PromptLibraryItem) => string | string[]) {
  const values = new Set<string>();
  items.forEach((item) => {
    const selected = selector(item);
    const list = Array.isArray(selected) ? selected : [selected];
    list.forEach((value) => {
      const trimmed = value.trim();
      if (trimmed) {
        values.add(trimmed);
      }
    });
  });
  return Array.from(values).sort((a, b) => a.localeCompare(b, "zh-Hans-CN"));
}

function PromptsPageContent({ session }: { session: StoredAuthSession }) {
  const navigate = useNavigate();
  const [items, setItems] = useState<PromptLibraryItem[]>([]);
  const [scope, setScope] = useState<PromptScope>("visible");
  const [searchKeyword, setSearchKeyword] = useState("");
  const [categoryFilter, setCategoryFilter] = useState(ALL_FILTER_VALUE);
  const [tagFilter, setTagFilter] = useState(ALL_FILTER_VALUE);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingPrompt, setEditingPrompt] = useState<PromptLibraryItem | null>(null);
  const [formState, setFormState] = useState<PromptFormState>(EMPTY_PROMPT_FORM);
  const [mutatingId, setMutatingId] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<PromptLibraryItem | null>(null);
  const canCreatePrompt = hasAPIPermission(session, "POST", "/api/prompts");

  const loadPrompts = useCallback(async () => {
    setIsLoading(true);
    setLoadError("");
    try {
      const data = await fetchPrompts({
        scope,
        q: searchKeyword.trim(),
        category: categoryFilter === ALL_FILTER_VALUE ? "" : categoryFilter,
        tag: tagFilter === ALL_FILTER_VALUE ? "" : tagFilter,
      });
      setItems(data);
    } catch (error) {
      const message = error instanceof Error ? error.message : "加载提示词失败";
      setLoadError(message);
      toast.error(message);
    } finally {
      setIsLoading(false);
    }
  }, [categoryFilter, scope, searchKeyword, tagFilter]);

  useEffect(() => {
    void loadPrompts();
  }, [loadPrompts]);

  const categories = useMemo(() => uniquePromptValues(items, (item) => item.category), [items]);
  const tags = useMemo(() => uniquePromptValues(items, (item) => item.tags), [items]);

  const openCreateDialog = () => {
    setEditingPrompt(null);
    setFormState(EMPTY_PROMPT_FORM);
    setDialogOpen(true);
  };

  const openEditDialog = (item: PromptLibraryItem) => {
    setEditingPrompt(item);
    setFormState(promptFormFromItem(item));
    setDialogOpen(true);
  };

  const updateForm = <K extends keyof PromptFormState>(key: K, value: PromptFormState[K]) => {
    setFormState((current) => ({ ...current, [key]: value }));
  };

  const submitPromptForm = async () => {
    const title = formState.title.trim();
    const body = formState.body.trim();
    if (!title || !body) {
      toast.error("标题和正文不能为空");
      return;
    }
    setMutatingId(editingPrompt?.id || "new");
    try {
      await savePrompt({
        id: editingPrompt?.id,
        title,
        body,
        tags: parsePromptTags(formState.tags),
        category: formState.category.trim(),
        note: formState.note.trim(),
        model: formState.model.trim(),
        use_case: formState.use_case.trim(),
        visibility: formState.visibility,
      });
      setDialogOpen(false);
      setEditingPrompt(null);
      toast.success(editingPrompt ? "提示词已更新" : "提示词已创建");
      await loadPrompts();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "保存提示词失败");
    } finally {
      setMutatingId(null);
    }
  };

  const handleCopyPromptToLibrary = async (item: PromptLibraryItem) => {
    setMutatingId(item.id);
    try {
      await copyPrompt(item.id);
      toast.success("已复制到我的提示词库");
      await loadPrompts();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "复制提示词失败");
    } finally {
      setMutatingId(null);
    }
  };

  const handleCopyPromptText = async (item: PromptLibraryItem) => {
    const body = item.body.trim();
    if (!body) {
      toast.error("提示词正文为空");
      return;
    }
    try {
      await copyTextToClipboard(body);
      toast.success("提示词正文已复制");
    } catch {
      toast.error("复制失败");
    }
  };

  const handleDeletePrompt = async () => {
    if (!deleteTarget) {
      return;
    }
    setMutatingId(deleteTarget.id);
    try {
      await deletePrompt(deleteTarget.id);
      toast.success("提示词已删除");
      setDeleteTarget(null);
      await loadPrompts();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "删除提示词失败");
    } finally {
      setMutatingId(null);
    }
  };

  const applyPromptToWorkspace = (item: PromptLibraryItem) => {
    window.sessionStorage.setItem(PENDING_PROMPT_STORAGE_KEY, item.body);
    navigate("/image");
  };

  return (
    <section className="flex flex-col gap-5 pb-20 sm:pb-24">
      <PageHeader
        eyebrow="Prompts"
        title="提示词库"
        actions={
          <Button type="button" className="h-10 rounded-full px-4" onClick={openCreateDialog} disabled={!canCreatePrompt}>
            <Plus className="size-4" />
            新建提示词
          </Button>
        }
      />

      <section className="grid gap-3 rounded-[18px] border border-border bg-background/80 p-3 shadow-[0_6px_20px_rgba(15,23,42,0.04)] sm:p-4 lg:grid-cols-[180px_minmax(0,1fr)_150px_150px]">
        <Select value={scope} onValueChange={(value) => setScope(value as PromptScope)}>
          <SelectTrigger className="h-10 rounded-lg">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value="visible">可见提示词</SelectItem>
              <SelectItem value="mine">我的提示词</SelectItem>
              <SelectItem value="public">公开提示词</SelectItem>
              {session.role === "admin" ? <SelectItem value="all">全部提示词</SelectItem> : null}
            </SelectGroup>
          </SelectContent>
        </Select>
        <div className="relative">
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={searchKeyword}
            onChange={(event) => setSearchKeyword(event.target.value)}
            placeholder="搜索标题、正文、标签、备注、模型"
            className="h-10 rounded-lg pr-9 pl-9"
          />
          {searchKeyword ? (
            <button
              type="button"
              className="absolute top-1/2 right-2 inline-flex size-6 -translate-y-1/2 items-center justify-center rounded-full text-muted-foreground transition hover:bg-muted hover:text-foreground"
              onClick={() => setSearchKeyword("")}
              aria-label="清空搜索"
              title="清空搜索"
            >
              <X className="size-3.5" />
            </button>
          ) : null}
        </div>
        <Select value={categoryFilter} onValueChange={setCategoryFilter}>
          <SelectTrigger className="h-10 rounded-lg">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value={ALL_FILTER_VALUE}>全部分类</SelectItem>
              {categories.map((category) => (
                <SelectItem key={category} value={category}>
                  {category}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Select value={tagFilter} onValueChange={setTagFilter}>
          <SelectTrigger className="h-10 rounded-lg">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              <SelectItem value={ALL_FILTER_VALUE}>全部标签</SelectItem>
              {tags.map((tag) => (
                <SelectItem key={tag} value={tag}>
                  {tag}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </section>

      {isLoading ? (
        <Card className="overflow-hidden rounded-[20px]">
          <CardContent className="flex min-h-[240px] items-center justify-center gap-2 text-sm text-muted-foreground">
            <LoaderCircle className="size-4 animate-spin text-[#1456f0]" />
            正在加载提示词
          </CardContent>
        </Card>
      ) : null}

      {!isLoading && loadError ? (
        <Card className="overflow-hidden rounded-[20px]">
          <CardContent className="flex min-h-[220px] flex-col items-center justify-center gap-3 text-center">
            <p className="text-sm font-medium text-foreground">提示词库加载失败</p>
            <p className="max-w-[32rem] text-sm leading-6 text-muted-foreground">{loadError}</p>
            <Button variant="outline" className="h-9 rounded-lg px-3" onClick={() => void loadPrompts()}>
              重试
            </Button>
          </CardContent>
        </Card>
      ) : null}

      {!isLoading && !loadError && items.length === 0 ? (
        <Card className="overflow-hidden rounded-[20px]">
          <CardContent className="flex min-h-[280px] flex-col items-center justify-center gap-4 px-6 py-14 text-center">
            <div className="rounded-[20px] bg-[#edf4ff] p-4 text-[#1456f0] ring-1 ring-blue-100">
              <Search className="size-7" />
            </div>
            <div className="space-y-1">
              <p className="text-sm font-medium text-foreground">暂无匹配的提示词</p>
              <p className="max-w-[32rem] text-sm leading-6 text-muted-foreground">可以新建提示词，或调整搜索、分类和标签筛选。</p>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {!isLoading && !loadError && items.length > 0 ? (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {items.map((item) => {
            const canEdit = canMutatePrompt(item, session);
            const canCopy = item.visibility === "public" && item.owner_id !== session.subjectId;
            const title = promptDisplayTitle(item);
            return (
              <article key={item.id} className="flex min-h-[240px] flex-col rounded-[18px] border border-border bg-background p-4 shadow-[0_6px_20px_rgba(15,23,42,0.04)]">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-semibold ${promptVisibilityPillClass(item.visibility)}`}>
                        {item.visibility === "public" ? <Globe2 className="size-3" /> : <Lock className="size-3" />}
                        {promptVisibilityLabel(item.visibility)}
                      </span>
                      {item.category ? (
                        <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
                          {item.category}
                        </span>
                      ) : null}
                    </div>
                    <h2 className="mt-2 line-clamp-2 text-base font-semibold leading-6 text-foreground">{title}</h2>
                    {shouldShowOriginalPromptTitle(item, title) ? (
                      <p className="mt-1 truncate text-[11px] text-muted-foreground">原标题：{item.title}</p>
                    ) : null}
                    <p className="mt-1 text-xs text-muted-foreground">作者：{item.owner_name || item.owner_id}</p>
                  </div>
                </div>
                <p className="mt-3 line-clamp-5 whitespace-pre-wrap text-sm leading-6 text-[#45515e] dark:text-muted-foreground">
                  {item.body}
                </p>
                {item.tags.length > 0 ? (
                  <div className="mt-3 flex flex-wrap gap-1.5">
                    {item.tags.slice(0, 6).map((tag) => (
                      <span key={tag} className="rounded-md bg-muted px-2 py-1 text-[11px] text-muted-foreground">
                        {tag}
                      </span>
                    ))}
                  </div>
                ) : null}
                <div className="mt-auto pt-4">
                  <div className="mb-3 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
                    {item.model ? <span>模型：{item.model}</span> : null}
                    {item.use_case ? <span>用途：{item.use_case}</span> : null}
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    <Button type="button" size="sm" className="rounded-lg" onClick={() => applyPromptToWorkspace(item)}>
                      <Send className="size-3.5" />
                      应用
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      className="rounded-lg"
                      onClick={() => void handleCopyPromptText(item)}
                    >
                      <Copy className="size-3.5" />
                      复制正文
                    </Button>
                    {canCopy ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        className="rounded-lg"
                        onClick={() => void handleCopyPromptToLibrary(item)}
                        disabled={mutatingId === item.id}
                      >
                        {mutatingId === item.id ? <LoaderCircle className="size-3.5 animate-spin" /> : <Copy className="size-3.5" />}
                        存入我的
                      </Button>
                    ) : null}
                    {canEdit ? (
                      <Button type="button" size="sm" variant="outline" className="rounded-lg" onClick={() => openEditDialog(item)}>
                        <Pencil className="size-3.5" />
                        编辑
                      </Button>
                    ) : null}
                    {canEdit ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        className="rounded-lg text-rose-600 hover:bg-rose-50 hover:text-rose-700"
                        onClick={() => setDeleteTarget(item)}
                        disabled={mutatingId === item.id}
                      >
                        <Trash2 className="size-3.5" />
                        删除
                      </Button>
                    ) : null}
                  </div>
                </div>
              </article>
            );
          })}
        </div>
      ) : null}

      <Dialog open={dialogOpen} onOpenChange={(open) => !mutatingId && setDialogOpen(open)}>
        <DialogContent className="max-h-[86vh] overflow-y-auto rounded-2xl p-6">
          <DialogHeader className="gap-2">
            <DialogTitle>{editingPrompt ? "编辑提示词" : "新建提示词"}</DialogTitle>
            <DialogDescription>维护标题、正文、标签、分类和可见性。</DialogDescription>
          </DialogHeader>
          <div className="grid gap-3">
            <Input value={formState.title} onChange={(event) => updateForm("title", event.target.value)} placeholder="标题" className="h-10 rounded-lg" />
            <Textarea
              value={formState.body}
              onChange={(event) => updateForm("body", event.target.value)}
              placeholder="提示词正文"
              className="min-h-40 rounded-xl text-sm leading-6"
            />
            <div className="grid gap-3 sm:grid-cols-2">
              <Input value={formState.category} onChange={(event) => updateForm("category", event.target.value)} placeholder="分类" className="h-10 rounded-lg" />
              <Input value={formState.tags} onChange={(event) => updateForm("tags", event.target.value)} placeholder="标签，逗号分隔" className="h-10 rounded-lg" />
              <Input value={formState.model} onChange={(event) => updateForm("model", event.target.value)} placeholder="适用模型" className="h-10 rounded-lg" />
              <Input value={formState.use_case} onChange={(event) => updateForm("use_case", event.target.value)} placeholder="用途" className="h-10 rounded-lg" />
            </div>
            <Textarea value={formState.note} onChange={(event) => updateForm("note", event.target.value)} placeholder="备注" className="min-h-24 rounded-xl text-sm leading-6" />
            <Select value={formState.visibility} onValueChange={(value) => updateForm("visibility", value as PromptVisibility)}>
              <SelectTrigger className="h-10 rounded-lg">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="private">私有</SelectItem>
                  <SelectItem value="public">公开</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDialogOpen(false)} disabled={Boolean(mutatingId)}>
              取消
            </Button>
            <Button type="button" onClick={() => void submitPromptForm()} disabled={Boolean(mutatingId)}>
              {mutatingId ? <LoaderCircle className="size-4 animate-spin" /> : null}
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {deleteTarget ? (
        <Dialog open onOpenChange={(open) => !open && !mutatingId ? setDeleteTarget(null) : null}>
          <DialogContent showCloseButton={false} className="rounded-2xl p-6">
            <DialogHeader className="gap-2">
              <DialogTitle>删除提示词</DialogTitle>
              <DialogDescription>确认删除“{promptDisplayTitle(deleteTarget)}”吗？删除后无法恢复。</DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDeleteTarget(null)} disabled={Boolean(mutatingId)}>
                取消
              </Button>
              <Button type="button" className="bg-rose-600 text-white hover:bg-rose-700" onClick={() => void handleDeletePrompt()} disabled={Boolean(mutatingId)}>
                {mutatingId === deleteTarget.id ? <LoaderCircle className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
                确认删除
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      ) : null}
    </section>
  );
}

export default function PromptsPage() {
  const { isCheckingAuth, session } = useAuthGuard(undefined, "/prompts");

  if (isCheckingAuth || !session) {
    return (
      <div className="flex min-h-[40vh] items-center justify-center">
        <LoaderCircle className="size-5 animate-spin text-stone-400" />
      </div>
    );
  }

  return <PromptsPageContent session={session} />;
}
