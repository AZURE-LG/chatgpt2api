"use client";

import {
  deleteImageConversationRemote,
  fetchConversationAttachmentBlobURL,
  fetchImageConversation,
  fetchImageConversations,
  saveImageConversationRemote,
} from "@/lib/image-workspace-api";
import type { ImageModel, ImageQuality, ImageVisibility } from "@/lib/api";

export type ImageConversationMode = "chat" | "generate" | "image" | "edit";
export type StoredReferenceImageSource = "upload" | "conversation";

export type StoredReferenceImage = {
  id?: string;
  name: string;
  type: string;
  dataUrl: string;
  url?: string;
  source?: StoredReferenceImageSource;
};

export type StoredImage = {
  id: string;
  taskId?: string;
  status?: "loading" | "success" | "error" | "cancelled" | "message";
  path?: string;
  visibility?: ImageVisibility;
  b64_json?: string;
  url?: string;
  revised_prompt?: string;
  error?: string;
  text_response?: string;
};

export type ImageTurnStatus = "queued" | "generating" | "success" | "error" | "cancelled" | "message";

export type ImageTurn = {
  id: string;
  prompt: string;
  model: ImageModel;
  mode: ImageConversationMode;
  referenceImages: StoredReferenceImage[];
  count: number;
  size: string;
  quality?: ImageQuality;
  visibility?: ImageVisibility;
  images: StoredImage[];
  createdAt: string;
  status: ImageTurnStatus;
  error?: string;
};

export type ImageConversation = {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  turns: ImageTurn[];
};

export type ImageConversationStats = {
  queued: number;
  running: number;
};

export const IMAGE_CONVERSATIONS_CHANGED_EVENT = "chatgpt2api:image-conversations-changed";
export const ACTIVE_IMAGE_CONVERSATION_STORAGE_KEY = "chatgpt2api:image_active_conversation_id";
export const IMAGE_ACTIVE_CONVERSATION_REQUEST_EVENT = "chatgpt2api:image-open-conversation";

let imageConversationWriteQueue: Promise<void> = Promise.resolve();

function dispatchImageConversationsChanged() {
  if (typeof window === "undefined") {
    return;
  }
  window.dispatchEvent(new Event(IMAGE_CONVERSATIONS_CHANGED_EVENT));
}

function sortImageConversations(conversations: ImageConversation[]): ImageConversation[] {
  return [...conversations].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
}

function queueImageConversationWrite<T>(operation: () => Promise<T>): Promise<T> {
  const result = imageConversationWriteQueue.then(operation);
  imageConversationWriteQueue = result.then(
    () => undefined,
    () => undefined,
  );
  return result;
}

function reusableReferencePreview(image: StoredReferenceImage | undefined) {
  const preview = image?.dataUrl || "";
  return preview.startsWith("data:") || preview.startsWith("blob:") ? preview : "";
}

export async function hydrateImageConversationReferencePreviews(
  conversation: ImageConversation,
  previewFallback?: ImageConversation,
): Promise<ImageConversation> {
  const turns = await Promise.all(
    conversation.turns.map(async (turn) => {
      const fallbackTurn = previewFallback?.turns.find((item) => item.id === turn.id);
      const referenceImages = await Promise.all(
        turn.referenceImages.map(async (image, index) => {
          const url = image.url || image.dataUrl;
          if (!url || !url.startsWith("/conversation-attachments/")) {
            return image;
          }
          try {
            return { ...image, dataUrl: await fetchConversationAttachmentBlobURL(url) };
          } catch {
            const fallbackPreview = reusableReferencePreview(fallbackTurn?.referenceImages[index]);
            if (fallbackPreview) {
              return { ...image, dataUrl: fallbackPreview };
            }
            return image;
          }
        }),
      );
      return { ...turn, referenceImages };
    }),
  );
  return { ...conversation, turns };
}

export async function listImageConversations(): Promise<ImageConversation[]> {
  const summaries = await fetchImageConversations();
  const hydrated = await Promise.all(
    summaries.map(async (summary) => {
      try {
        return await hydrateImageConversationReferencePreviews(await fetchImageConversation(summary.id));
      } catch {
        return summary;
      }
    }),
  );
  return sortImageConversations(hydrated);
}

export async function saveImageConversations(conversations: ImageConversation[]): Promise<ImageConversation[]> {
  return queueImageConversationWrite(async () => {
    const saved = await Promise.all(
      conversations.map(async (conversation) =>
        hydrateImageConversationReferencePreviews(await saveImageConversationRemote(conversation), conversation),
      ),
    );
    dispatchImageConversationsChanged();
    return saved;
  });
}

export async function saveImageConversation(conversation: ImageConversation): Promise<ImageConversation> {
  return queueImageConversationWrite(async () => {
    const saved = await hydrateImageConversationReferencePreviews(await saveImageConversationRemote(conversation), conversation);
    dispatchImageConversationsChanged();
    return saved;
  });
}

export async function deleteImageConversation(id: string): Promise<void> {
  await queueImageConversationWrite(async () => {
    await deleteImageConversationRemote(id);
    dispatchImageConversationsChanged();
  });
}

export async function clearImageConversations(): Promise<void> {
  await queueImageConversationWrite(async () => {
    const items = await fetchImageConversations();
    await Promise.all(items.map((item) => deleteImageConversationRemote(item.id)));
    dispatchImageConversationsChanged();
  });
}

export function getImageConversationStats(conversation: ImageConversation | null): ImageConversationStats {
  if (!conversation) {
    return { queued: 0, running: 0 };
  }

  return conversation.turns.reduce(
    (acc, turn) => {
      if (turn.status === "queued") {
        acc.queued += 1;
      } else if (turn.status === "generating") {
        acc.running += 1;
      }
      return acc;
    },
    { queued: 0, running: 0 },
  );
}
