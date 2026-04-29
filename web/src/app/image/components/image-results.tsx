"use client";

import { useState } from "react";
import { CircleStop, Clock3, LoaderCircle, PencilLine, RotateCcw, Sparkles } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ImageConversation, ImageTurn, ImageTurnStatus, StoredImage, StoredReferenceImage } from "@/store/image-conversations";

export type ImageLightboxItem = {
  id: string;
  src: string;
  sizeLabel?: string;
  dimensions?: string;
};

type ImageResultsProps = {
  selectedConversation: ImageConversation | null;
  onOpenLightbox: (images: ImageLightboxItem[], index: number) => void;
  onContinueEdit: (conversationId: string, image: StoredImage | StoredReferenceImage) => void;
  onEditTurn: (conversationId: string, turnId: string) => void;
  onCancelTurn: (conversationId: string, turnId: string) => void | Promise<void>;
  onRegenerateTurn: (conversationId: string, turnId: string) => void | Promise<void>;
  formatConversationTime: (value: string) => string;
};

function getStoredImageSrc(image: StoredImage) {
  if (image.b64_json) {
    return `data:image/png;base64,${image.b64_json}`;
  }
  return image.url || "";
}

function isTurnBusy(turn: ImageTurn) {
  return (
    turn.status === "queued" ||
    turn.status === "generating" ||
    turn.images.some((image) => image.status === "loading")
  );
}

export function ImageResults({
  selectedConversation,
  onOpenLightbox,
  onContinueEdit,
  onEditTurn,
  onCancelTurn,
  onRegenerateTurn,
  formatConversationTime,
}: ImageResultsProps) {
  const [imageDimensions, setImageDimensions] = useState<Record<string, string>>({});

  const updateImageDimensions = (id: string, width: number, height: number) => {
    const dimensions = formatImageDimensions(width, height);
    setImageDimensions((current) => {
      if (current[id] === dimensions) {
        return current;
      }
      return { ...current, [id]: dimensions };
    });
  };

  if (!selectedConversation) {
    return (
      <div className="flex h-full min-h-[260px] items-center justify-center text-center sm:min-h-[420px]">
        <div className="w-full max-w-4xl">
          <h1
            className="text-2xl font-semibold tracking-tight text-stone-950 sm:text-3xl md:text-5xl"
            style={{
              fontFamily: '"Palatino Linotype","Book Antiqua","URW Palladio L","Times New Roman",serif',
            }}
          >
            Turn ideas into images
          </h1>
          <p
            className="mx-auto mt-3 max-w-[280px] text-sm italic tracking-[0.01em] text-stone-500 sm:mt-4 sm:max-w-none sm:text-[15px]"
            style={{
              fontFamily: '"Palatino Linotype","Book Antiqua","URW Palladio L","Times New Roman",serif',
            }}
          >
            在同一窗口里保留本地历史与任务状态，并从已有结果图继续发起新的无状态编辑。
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto flex w-full max-w-[980px] flex-col gap-5 sm:gap-8">
      {selectedConversation.turns.map((turn, turnIndex) => {
        const referenceLightboxImages = turn.referenceImages.map((image, index) => ({
          id: `${turn.id}-reference-${index}`,
          src: image.dataUrl,
        }));
        const successfulTurnImages = turn.images.flatMap((image) => {
          const src = image.status === "success" ? getStoredImageSrc(image) : "";
          return src
            ? [
                {
                  id: image.id,
                  src,
                  sizeLabel: image.b64_json ? formatBase64ImageSize(image.b64_json) : undefined,
                  dimensions: imageDimensions[image.id],
                },
              ]
            : [];
        });
        const turnBusy = isTurnBusy(turn);
        const successCount = turn.images.filter((image) => image.status === "success").length;
        const failedCount = turn.images.filter((image) => image.status === "error").length;
        const cancelledCount = turn.images.filter((image) => image.status === "cancelled").length;
        const resultCount = turn.images.length || turn.count;
        const outcomeLabel = getTurnOutcomeLabel(successCount, failedCount, cancelledCount);

        return (
          <div key={turn.id} className="flex flex-col gap-3 sm:gap-4">
            <div className="flex justify-end">
              <article className="w-full max-w-[min(94%,760px)] rounded-[22px] border border-stone-200/80 bg-white px-4 py-3 text-left text-[14px] leading-6 text-stone-900 shadow-sm sm:px-5 sm:py-4 sm:text-[15px] sm:leading-7">
                <div className="mb-3 flex items-start justify-between gap-3 border-b border-stone-100 pb-2">
                  <div className="flex min-w-0 flex-wrap items-center gap-1.5 text-[11px] leading-5 text-stone-500">
                    <span className="rounded-full bg-stone-100 px-2.5 py-0.5 text-stone-600">第 {turnIndex + 1} 轮</span>
                    <span className="rounded-full bg-stone-100 px-2.5 py-0.5 text-stone-600">{getTurnModeLabel(turn)}</span>
                    <span className="rounded-full bg-stone-100 px-2.5 py-0.5 text-stone-600">{turn.model}</span>
                    <span className="rounded-full bg-stone-100 px-2.5 py-0.5 text-stone-600">
                      {getTurnStatusLabel(turn.status)}
                    </span>
                    <span className="px-1 text-stone-400">{formatConversationTime(turn.createdAt)}</span>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    {turnBusy ? (
                      <Button
                        type="button"
                        variant="outline"
                        size="icon"
                        className="size-8 rounded-full border-amber-200 bg-amber-50 text-amber-700 shadow-none hover:bg-amber-100"
                        onClick={() => void onCancelTurn(selectedConversation.id, turn.id)}
                        aria-label="终止生成任务"
                        title="终止"
                      >
                        <CircleStop className="size-4" />
                      </Button>
                    ) : (
                      <>
                        <Button
                          type="button"
                          variant="outline"
                          size="icon"
                          className="size-8 rounded-full border-stone-200 bg-white text-stone-600 shadow-none hover:bg-stone-50"
                          onClick={() => onEditTurn(selectedConversation.id, turn.id)}
                          aria-label="编辑生成设置"
                          title="编辑"
                        >
                          <PencilLine className="size-4" />
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          size="icon"
                          className="size-8 rounded-full border-stone-200 bg-white text-stone-600 shadow-none hover:bg-stone-50"
                          disabled={turnBusy || !turn.prompt.trim()}
                          onClick={() => void onRegenerateTurn(selectedConversation.id, turn.id)}
                          aria-label="重新生成"
                          title="重新生成"
                        >
                          <RotateCcw className="size-4" />
                        </Button>
                      </>
                    )}
                  </div>
                </div>
                <div>
                  <div className="whitespace-pre-wrap break-words">{turn.prompt}</div>
                  {turn.referenceImages.length > 0 ? (
                    <div className="mt-3 flex flex-wrap justify-start gap-2">
                      {turn.referenceImages.map((image, index) => (
                        <button
                          key={`${turn.id}-${image.name}-${index}`}
                          type="button"
                          onClick={() => onOpenLightbox(referenceLightboxImages, index)}
                          className="group relative size-20 shrink-0 overflow-hidden rounded-2xl border border-stone-200/80 bg-stone-100/60 text-left transition hover:border-stone-300 sm:size-24"
                          aria-label={`预览参考图 ${image.name || index + 1}`}
                        >
                          <img
                            src={image.dataUrl}
                            alt={image.name || `参考图 ${index + 1}`}
                            className="absolute inset-0 h-full w-full object-cover transition duration-200 group-hover:scale-[1.02]"
                          />
                        </button>
                      ))}
                    </div>
                  ) : null}
                </div>
              </article>
            </div>

            <div className="flex justify-start">
              <section className="w-full px-1">
                <div className="mb-3 flex flex-col gap-2 sm:mb-4 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-stone-500 sm:gap-2 sm:text-xs">
                    <span className="font-medium text-stone-700">生成结果</span>
                    <span className="rounded-full bg-stone-100 px-3 py-1">{resultCount} 张</span>
                    {turn.count !== resultCount ? (
                      <span className="rounded-full bg-stone-100 px-3 py-1">目标 {turn.count} 张</span>
                    ) : null}
                    {turn.size ? <span className="rounded-full bg-stone-100 px-3 py-1">{turn.size}</span> : null}
                    {outcomeLabel ? <span className="rounded-full bg-stone-100 px-3 py-1">{outcomeLabel}</span> : null}
                    <span className={cn("rounded-full px-3 py-1", getStatusChipClass(turn.status))}>
                      {getTurnStatusLabel(turn.status)}
                    </span>
                  </div>
                  {turn.status === "queued" ? (
                    <span className="w-fit rounded-full bg-amber-50 px-3 py-1 text-[11px] text-amber-700 sm:text-xs">
                      等待前序任务
                    </span>
                  ) : null}
                </div>

                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 sm:gap-4 xl:grid-cols-3">
                  {turn.images.map((image, index) => {
                    const imageSrc = image.status === "success" ? getStoredImageSrc(image) : "";
                    if (image.status === "success" && imageSrc) {
                      const currentIndex = successfulTurnImages.findIndex((item) => item.id === image.id);
                      const sizeLabel = image.b64_json ? formatBase64ImageSize(image.b64_json) : "";
                      const dimensions = imageDimensions[image.id];
                      const imageMeta = [sizeLabel, dimensions].filter(Boolean).join(" · ");

                      return (
                        <figure
                          key={image.id}
                          className="overflow-hidden rounded-[18px] border border-stone-200/80 bg-white shadow-sm"
                        >
                          <button
                            type="button"
                            onClick={() => onOpenLightbox(successfulTurnImages, currentIndex)}
                            className="group block w-full cursor-zoom-in overflow-hidden bg-stone-50"
                          >
                            <img
                              src={imageSrc}
                              alt={`Generated result ${index + 1}`}
                              className="block h-auto w-full transition duration-200 group-hover:brightness-95"
                              onLoad={(event) => {
                                updateImageDimensions(
                                  image.id,
                                  event.currentTarget.naturalWidth,
                                  event.currentTarget.naturalHeight,
                                );
                              }}
                            />
                          </button>
                          <figcaption className="flex min-h-12 items-center justify-between gap-2 border-t border-stone-100 px-3 py-2.5">
                            <div className="min-w-0 text-xs text-stone-500">
                              <div className="font-medium text-stone-600">结果 {index + 1}</div>
                              {imageMeta ? <div className="mt-0.5 truncate text-stone-400">{imageMeta}</div> : null}
                            </div>
                            <Button
                              variant="outline"
                              size="sm"
                              className="h-8 shrink-0 rounded-full border-stone-200 bg-white px-3 text-xs text-stone-700 hover:bg-stone-50"
                              onClick={() => onContinueEdit(selectedConversation.id, image)}
                            >
                              <Sparkles className="size-3.5" />
                              加入编辑
                            </Button>
                          </figcaption>
                        </figure>
                      );
                    }

                    if (image.status === "cancelled") {
                      return (
                        <div
                          key={image.id}
                          className="min-h-[180px] overflow-hidden rounded-[18px] border border-amber-200 bg-amber-50"
                        >
                          <div className="flex h-full min-h-16 items-center justify-center px-4 py-4 text-center text-sm leading-6 text-amber-700 sm:px-6 sm:py-8">
                            {image.error || "任务已终止"}
                          </div>
                        </div>
                      );
                    }

                    if (image.status === "error") {
                      return (
                        <div
                          key={image.id}
                          className="min-h-[180px] overflow-hidden rounded-[18px] border border-rose-200 bg-rose-50"
                        >
                          <div className="flex h-full min-h-16 items-center justify-center px-4 py-4 text-center text-sm leading-6 text-rose-600 sm:px-6 sm:py-8">
                            {image.error || "生成失败"}
                          </div>
                        </div>
                      );
                    }

                    return (
                      <div
                        key={image.id}
                        className="min-h-[180px] overflow-hidden rounded-[18px] border border-stone-200/80 bg-stone-100/80"
                      >
                        <div className="flex h-full flex-col items-center justify-center gap-3 px-6 py-8 text-center text-stone-500">
                          <div className="rounded-full bg-white p-3 shadow-sm">
                            {turn.status === "queued" ? (
                              <Clock3 className="size-5" />
                            ) : (
                              <LoaderCircle className="size-5 animate-spin" />
                            )}
                          </div>
                          <p className="text-sm">{turn.status === "queued" ? "已加入当前对话队列..." : "正在处理图片..."}</p>
                        </div>
                      </div>
                    );
                  })}
                </div>

                {turn.status === "error" && turn.error ? (
                  <div className="mt-4 border-l-2 border-amber-300 bg-amber-50/70 px-4 py-3 text-sm leading-6 text-amber-700">
                    {turn.error}
                  </div>
                ) : null}
              </section>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function getTurnStatusLabel(status: ImageTurnStatus) {
  if (status === "queued") {
    return "排队中";
  }
  if (status === "generating") {
    return "处理中";
  }
  if (status === "success") {
    return "已完成";
  }
  if (status === "cancelled") {
    return "已终止";
  }
  return "失败";
}

function getStatusChipClass(status: ImageTurnStatus) {
  if (status === "queued") {
    return "bg-amber-50 text-amber-700";
  }
  if (status === "generating") {
    return "bg-blue-50 text-blue-700";
  }
  if (status === "success") {
    return "bg-emerald-50 text-emerald-700";
  }
  if (status === "cancelled") {
    return "bg-amber-50 text-amber-700";
  }
  return "bg-rose-50 text-rose-700";
}

function getTurnOutcomeLabel(successCount: number, failedCount: number, cancelledCount: number) {
  if (failedCount === 0 && cancelledCount === 0) {
    return "";
  }
  const parts = [`成功 ${successCount}`];
  if (failedCount > 0) {
    parts.push(`失败 ${failedCount}`);
  }
  if (cancelledCount > 0) {
    parts.push(`终止 ${cancelledCount}`);
  }
  return parts.join(" / ");
}

function getTurnModeLabel(turn: ImageTurn) {
  if (turn.mode === "generate") {
    return "文生图";
  }
  if (turn.mode === "edit" && turn.referenceImages.some((image) => image.source === "conversation")) {
    return "编辑图";
  }
  return "图生图";
}

function formatBase64ImageSize(base64: string) {
  const normalized = base64.replace(/\s/g, "");
  const padding = normalized.endsWith("==") ? 2 : normalized.endsWith("=") ? 1 : 0;
  const bytes = Math.max(0, Math.floor((normalized.length * 3) / 4) - padding);

  if (bytes >= 1024 * 1024) {
    return `${(bytes / 1024 / 1024).toFixed(2)} MB`;
  }
  if (bytes >= 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  return `${bytes} B`;
}

function formatImageDimensions(width: number, height: number) {
  return `${width} x ${height}`;
}
