"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Ban,
  CheckCircle2,
  Copy,
  Eye,
  EyeOff,
  LoaderCircle,
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  Trash2,
  X,
} from "lucide-react";
import { toast } from "sonner";

import { PageHeader } from "@/components/page-header";
import { Badge } from "@/components/ui/badge";
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  createManagedUser,
  deleteManagedUser,
  fetchManagedUsers,
  resetManagedUserKey,
  revealManagedUserKey,
  updateManagedUser,
  type ManagedUser,
} from "@/lib/api";
import { useAuthGuard } from "@/lib/use-auth-guard";

function formatDateTime(value?: string | null) {
  if (!value) {
    return "—";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function normalizeManagedUsers(items: ManagedUser[] | null | undefined) {
  return Array.isArray(items) ? items : [];
}

function providerLabel(provider?: string) {
  if (provider === "linuxdo") {
    return "Linuxdo";
  }
  if (provider === "local") {
    return "本地";
  }
  return provider || "未知";
}

function maskToken(hasToken: boolean) {
  return hasToken ? "••••••••••••••••••••••••" : "未生成";
}

function numeric(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function todayQuotaUsed(user: ManagedUser) {
  const points = Array.isArray(user.usage_curve) ? user.usage_curve : [];
  if (points.length === 0) {
    return 0;
  }
  return numeric(points[points.length - 1]?.quota_used);
}

function UsageSparkline({ points }: { points?: ManagedUser["usage_curve"] }) {
  const safePoints = Array.isArray(points) ? points : [];
  const maxCalls = Math.max(1, ...safePoints.map((point) => numeric(point.calls)));

  return (
    <div className="flex h-12 w-[170px] items-end gap-1" aria-label="调用曲线">
      {safePoints.map((point) => {
        const calls = numeric(point.calls);
        const height = Math.max(4, Math.round((calls / maxCalls) * 40));
        return (
          <div
            key={point.date}
            className="w-2 rounded-t-sm bg-sky-500/70 dark:bg-sky-400/70"
            style={{ height }}
            title={`${point.date} 调用 ${calls} 次，额度 ${numeric(point.quota_used)}`}
          />
        );
      })}
    </div>
  );
}

function userSearchText(user: ManagedUser) {
  return [
    user.id,
    user.name,
    user.owner_id,
    user.owner_name,
    user.provider,
    user.api_key_id,
    user.api_key_name,
    user.session_id,
    user.session_name,
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
}

function UsersContent() {
  const [items, setItems] = useState<ManagedUser[]>([]);
  const [searchText, setSearchText] = useState("");
  const [providerFilter, setProviderFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState("all");
  const [isLoading, setIsLoading] = useState(true);
  const [pendingIds, setPendingIds] = useState<Set<string>>(() => new Set());
  const [revealingIds, setRevealingIds] = useState<Set<string>>(() => new Set());
  const [revealedKeysById, setRevealedKeysById] = useState<Record<string, string>>({});
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);
  const [createName, setCreateName] = useState("");
  const [isCreating, setIsCreating] = useState(false);
  const [resettingUser, setResettingUser] = useState<ManagedUser | null>(null);
  const [resetName, setResetName] = useState("");
  const [deletingUser, setDeletingUser] = useState<ManagedUser | null>(null);

  const loadUsers = useCallback(async () => {
    setIsLoading(true);
    try {
      const data = await fetchManagedUsers();
      setItems(normalizeManagedUsers(data.items));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "加载用户失败");
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadUsers();
  }, [loadUsers]);

  const filteredItems = useMemo(() => {
    const keyword = searchText.trim().toLowerCase();
    return items.filter((user) => {
      if (providerFilter !== "all" && user.provider !== providerFilter) {
        return false;
      }
      if (statusFilter === "enabled" && !user.enabled) {
        return false;
      }
      if (statusFilter === "disabled" && user.enabled) {
        return false;
      }
      return !keyword || userSearchText(user).includes(keyword);
    });
  }, [items, providerFilter, searchText, statusFilter]);
  const hasActiveFilters = searchText.trim() !== "" || providerFilter !== "all" || statusFilter !== "all";

  const setItemPending = (id: string, isPending: boolean) => {
    setPendingIds((current) => {
      const next = new Set(current);
      if (isPending) {
        next.add(id);
      } else {
        next.delete(id);
      }
      return next;
    });
  };

  const setRevealPending = (id: string, isPending: boolean) => {
    setRevealingIds((current) => {
      const next = new Set(current);
      if (isPending) {
        next.add(id);
      } else {
        next.delete(id);
      }
      return next;
    });
  };

  const handleCreate = async () => {
    setIsCreating(true);
    try {
      const data = await createManagedUser(createName.trim());
      setItems(normalizeManagedUsers(data.items));
      setRevealedKeysById((current) => ({ ...current, [data.item.id]: data.key }));
      setCreateName("");
      setIsCreateDialogOpen(false);
      toast.success("用户已创建");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "创建用户失败");
    } finally {
      setIsCreating(false);
    }
  };

  const handleToggle = async (user: ManagedUser) => {
    setItemPending(user.id, true);
    try {
      const data = await updateManagedUser(user.id, { enabled: !user.enabled });
      setItems(normalizeManagedUsers(data.items));
      toast.success(user.enabled ? "用户已禁用" : "用户已启用");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "更新用户失败");
    } finally {
      setItemPending(user.id, false);
    }
  };

  const handleReveal = async (user: ManagedUser) => {
    if (revealedKeysById[user.id]) {
      setRevealedKeysById((current) => {
        const next = { ...current };
        delete next[user.id];
        return next;
      });
      return;
    }
    if (!user.has_api_key) {
      toast.error("该用户还没有 API 令牌");
      return;
    }

    setRevealPending(user.id, true);
    try {
      const data = await revealManagedUserKey(user.id);
      setRevealedKeysById((current) => ({ ...current, [user.id]: data.key }));
      toast.success("令牌已显示");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "查看令牌失败");
    } finally {
      setRevealPending(user.id, false);
    }
  };

  const openResetDialog = (user: ManagedUser) => {
    setResetName(user.api_key_name || "");
    setResettingUser(user);
  };

  const handleReset = async () => {
    if (!resettingUser) {
      return;
    }
    const user = resettingUser;
    setItemPending(user.id, true);
    try {
      const data = await resetManagedUserKey(user.id, resetName.trim());
      setItems(normalizeManagedUsers(data.items));
      setRevealedKeysById((current) => ({ ...current, [user.id]: data.key }));
      setResettingUser(null);
      setResetName("");
      toast.success(user.has_api_key ? "令牌已重置" : "令牌已生成");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "重置令牌失败");
    } finally {
      setItemPending(user.id, false);
    }
  };

  const handleDelete = async () => {
    if (!deletingUser) {
      return;
    }
    const user = deletingUser;
    setItemPending(user.id, true);
    try {
      const data = await deleteManagedUser(user.id);
      setItems(normalizeManagedUsers(data.items));
      setDeletingUser(null);
      setRevealedKeysById((current) => {
        const next = { ...current };
        delete next[user.id];
        return next;
      });
      toast.success("用户已删除");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "删除用户失败");
    } finally {
      setItemPending(user.id, false);
    }
  };

  const handleCopy = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast.success("已复制到剪贴板");
    } catch {
      toast.error("复制失败，请手动复制");
    }
  };

  return (
    <section className="flex flex-col gap-5">
      <PageHeader
        eyebrow="Users"
        title="用户管理"
        actions={
          <>
            <Button variant="outline" onClick={() => void loadUsers()} disabled={isLoading} className="h-10 rounded-lg">
              <RefreshCw className={`size-4 ${isLoading ? "animate-spin" : ""}`} />
              刷新
            </Button>
            <Button onClick={() => setIsCreateDialogOpen(true)} className="h-10 rounded-lg">
              <Plus className="size-4" />
              创建用户
            </Button>
          </>
        }
      />

      <Card className="overflow-hidden">
        <CardContent className="p-0">
          <div className="flex flex-col gap-3 border-b border-border px-5 py-4">
            <div className="flex items-center justify-between text-sm text-muted-foreground">
              <span>共 {filteredItems.length} / {items.length} 个用户</span>
            </div>
            <div className="grid gap-2 lg:grid-cols-[minmax(18rem,1fr)_160px_160px_auto]">
              <div className="relative">
                <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={searchText}
                  onChange={(event) => setSearchText(event.target.value)}
                  placeholder="搜索用户名、用户 ID、owner、令牌或会话"
                  className="h-10 rounded-lg pl-9"
                />
              </div>
              <Select value={providerFilter} onValueChange={setProviderFilter}>
                <SelectTrigger className="h-10 rounded-lg">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">全部来源</SelectItem>
                  <SelectItem value="linuxdo">Linuxdo</SelectItem>
                  <SelectItem value="local">本地</SelectItem>
                </SelectContent>
              </Select>
              <Select value={statusFilter} onValueChange={setStatusFilter}>
                <SelectTrigger className="h-10 rounded-lg">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">全部状态</SelectItem>
                  <SelectItem value="enabled">已启用</SelectItem>
                  <SelectItem value="disabled">已禁用</SelectItem>
                </SelectContent>
              </Select>
              <Button
                type="button"
                variant="outline"
                className="h-10 rounded-lg px-3"
                disabled={!hasActiveFilters}
                onClick={() => {
                  setSearchText("");
                  setProviderFilter("all");
                  setStatusFilter("all");
                }}
              >
                <X className="size-4" />
                清除
              </Button>
            </div>
          </div>
          <div className="overflow-x-auto">
            <Table className="min-w-[1220px]">
              <TableHeader>
                <TableRow>
                  <TableHead>用户</TableHead>
                  <TableHead>来源</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>额度消耗</TableHead>
                  <TableHead>调用曲线</TableHead>
                  <TableHead>令牌</TableHead>
                  <TableHead>时间</TableHead>
                  <TableHead className="w-[280px]">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredItems.map((user) => {
                  const isPending = pendingIds.has(user.id);
                  const isRevealing = revealingIds.has(user.id);
                  const revealedKey = revealedKeysById[user.id] ?? "";
                  const canManageToken = user.provider !== "linuxdo";
                  return (
                    <TableRow key={user.id} className="text-muted-foreground">
                      <TableCell>
                        <div className="min-w-0 space-y-1">
                          <div className="truncate font-medium text-foreground">{user.name || "普通用户"}</div>
                          <code className="block max-w-[260px] truncate font-mono text-xs text-muted-foreground">{user.id}</code>
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-col items-start gap-1.5">
                          <Badge variant={user.provider === "linuxdo" ? "info" : "secondary"} className="rounded-md">
                            {providerLabel(user.provider)}
                          </Badge>
                          {user.owner_name ? <span className="text-xs text-muted-foreground">{user.owner_name}</span> : null}
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant={user.enabled ? "success" : "danger"} className="rounded-md">
                          {user.enabled ? "已启用" : "已禁用"}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <div className="space-y-1">
                          <div className="text-base font-semibold text-foreground">{numeric(user.quota_used)}</div>
                          <div className="text-xs text-muted-foreground">今日 {todayQuotaUsed(user)}</div>
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-3">
                          <UsageSparkline points={user.usage_curve} />
                          <div className="space-y-1 text-xs text-muted-foreground">
                            <div>调用 {numeric(user.call_count)}</div>
                            <div>失败 {numeric(user.failure_count)}</div>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                        {canManageToken ? (
                          <div className="flex max-w-[300px] items-center gap-2 rounded-lg border border-border bg-muted/40 px-3 py-2">
                            <code className="min-w-0 flex-1 truncate font-mono text-xs text-foreground">
                              {revealedKey || maskToken(user.has_api_key)}
                            </code>
                            {revealedKey ? (
                              <Button
                                type="button"
                                variant="ghost"
                                size="icon"
                                className="size-7 rounded-lg"
                                onClick={() => void handleCopy(revealedKey)}
                                aria-label="复制令牌"
                              >
                                <Copy className="size-3.5" />
                              </Button>
                            ) : null}
                          </div>
                        ) : (
                          <Badge variant="secondary" className="rounded-md">
                            Linuxdo 登录
                          </Badge>
                        )}
                      </TableCell>
                      <TableCell>
                        <div className="space-y-1 text-xs">
                          <div>创建 {formatDateTime(user.created_at)}</div>
                          <div>使用 {formatDateTime(user.last_used_at)}</div>
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-wrap justify-end gap-2">
                          {canManageToken ? (
                            <>
                              <Button
                                type="button"
                                variant="outline"
                                className="h-8 rounded-lg px-3"
                                onClick={() => void handleReveal(user)}
                                disabled={isRevealing || !user.has_api_key}
                              >
                                {isRevealing ? (
                                  <LoaderCircle className="size-4 animate-spin" />
                                ) : revealedKey ? (
                                  <EyeOff className="size-4" />
                                ) : (
                                  <Eye className="size-4" />
                                )}
                                {revealedKey ? "隐藏" : "查看"}
                              </Button>
                              <Button
                                type="button"
                                variant="outline"
                                className="h-8 rounded-lg px-3"
                                onClick={() => openResetDialog(user)}
                                disabled={isPending}
                              >
                                <RotateCcw className="size-4" />
                                {user.has_api_key ? "重置" : "生成"}
                              </Button>
                            </>
                          ) : null}
                          <Button
                            type="button"
                            variant="outline"
                            className="h-8 rounded-lg px-3"
                            onClick={() => void handleToggle(user)}
                            disabled={isPending}
                          >
                            {isPending ? (
                              <LoaderCircle className="size-4 animate-spin" />
                            ) : user.enabled ? (
                              <Ban className="size-4" />
                            ) : (
                              <CheckCircle2 className="size-4" />
                            )}
                            {user.enabled ? "禁用" : "启用"}
                          </Button>
                          <Button
                            type="button"
                            variant="outline"
                            className="h-8 rounded-lg border-rose-200 px-3 text-rose-600 hover:bg-rose-50 hover:text-rose-700"
                            onClick={() => setDeletingUser(user)}
                            disabled={isPending}
                          >
                            <Trash2 className="size-4" />
                            删除
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
          {isLoading ? (
            <div className="flex items-center justify-center py-14">
              <LoaderCircle className="size-5 animate-spin text-stone-400" />
            </div>
          ) : null}
          {!isLoading && filteredItems.length === 0 ? <div className="px-6 py-14 text-center text-sm text-stone-500">{items.length === 0 ? "暂无用户" : "没有匹配的用户"}</div> : null}
        </CardContent>
      </Card>

      <Dialog open={isCreateDialogOpen} onOpenChange={setIsCreateDialogOpen}>
        <DialogContent className="rounded-2xl p-6">
          <DialogHeader className="gap-2">
            <DialogTitle>创建用户</DialogTitle>
            <DialogDescription className="text-sm leading-6">创建后会立即生成一条 API 令牌。</DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <label className="text-sm font-medium text-stone-700 dark:text-foreground">名称</label>
            <Input
              value={createName}
              onChange={(event) => setCreateName(event.target.value)}
              placeholder="例如：运营账号"
              className="h-11 rounded-xl"
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="secondary" className="h-10 rounded-xl px-5" onClick={() => setIsCreateDialogOpen(false)} disabled={isCreating}>
              取消
            </Button>
            <Button type="button" className="h-10 rounded-xl px-5" onClick={() => void handleCreate()} disabled={isCreating}>
              {isCreating ? <LoaderCircle className="size-4 animate-spin" /> : <Plus className="size-4" />}
              创建
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(resettingUser)} onOpenChange={(open) => (!open ? setResettingUser(null) : null)}>
        <DialogContent className="rounded-2xl p-6">
          <DialogHeader className="gap-2">
            <DialogTitle>{resettingUser?.has_api_key ? "重置令牌" : "生成令牌"}</DialogTitle>
            <DialogDescription className="text-sm leading-6">
              {resettingUser?.has_api_key ? `确认重置「${resettingUser.name}」的 API 令牌吗？` : `为「${resettingUser?.name}」生成 API 令牌。`}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <label className="text-sm font-medium text-stone-700 dark:text-foreground">令牌名称</label>
            <Input
              value={resetName}
              onChange={(event) => setResetName(event.target.value)}
              placeholder="我的 API 令牌"
              className="h-11 rounded-xl"
            />
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="secondary"
              className="h-10 rounded-xl px-5"
              onClick={() => setResettingUser(null)}
              disabled={resettingUser ? pendingIds.has(resettingUser.id) : false}
            >
              取消
            </Button>
            <Button
              type="button"
              className="h-10 rounded-xl px-5"
              onClick={() => void handleReset()}
              disabled={resettingUser ? pendingIds.has(resettingUser.id) : false}
            >
              {resettingUser && pendingIds.has(resettingUser.id) ? <LoaderCircle className="size-4 animate-spin" /> : <RotateCcw className="size-4" />}
              确认
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={Boolean(deletingUser)} onOpenChange={(open) => (!open ? setDeletingUser(null) : null)}>
        <DialogContent className="rounded-2xl p-6">
          <DialogHeader className="gap-2">
            <DialogTitle>删除用户</DialogTitle>
            <DialogDescription className="text-sm leading-6">确认删除「{deletingUser?.name}」吗？</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="secondary"
              className="h-10 rounded-xl px-5"
              onClick={() => setDeletingUser(null)}
              disabled={deletingUser ? pendingIds.has(deletingUser.id) : false}
            >
              取消
            </Button>
            <Button
              type="button"
              className="h-10 rounded-xl bg-rose-600 px-5 text-white hover:bg-rose-700"
              onClick={() => void handleDelete()}
              disabled={deletingUser ? pendingIds.has(deletingUser.id) : false}
            >
              {deletingUser && pendingIds.has(deletingUser.id) ? <LoaderCircle className="size-4 animate-spin" /> : <Trash2 className="size-4" />}
              删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

export default function UsersPage() {
  const { isCheckingAuth, session } = useAuthGuard(["admin"]);
  if (isCheckingAuth || !session || session.role !== "admin") {
    return <div className="flex min-h-[40vh] items-center justify-center"><LoaderCircle className="size-5 animate-spin text-stone-400" /></div>;
  }
  return <UsersContent />;
}
