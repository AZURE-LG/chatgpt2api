import { httpRequest } from "@/lib/request";
import webConfig from "@/constants/common-env";
import { getStoredSessionToken } from "@/store/auth";
import type { ManagedImage } from "@/lib/api";
import type {
  ImageConversation,
  ImageConversationMode,
  ImageTurn,
  ImageTurnStatus,
  StoredImage,
  StoredReferenceImage,
} from "@/store/image-conversations";

type RemoteReferenceImage = {
  id?: string;
  name?: string;
  type?: string;
  url?: string;
  source?: string;
};

type RemoteResultImage = {
  id?: string;
  task_id?: string;
  status?: StoredImage["status"];
  path?: string;
  visibility?: StoredImage["visibility"];
  b64_json?: string;
  url?: string;
  revised_prompt?: string;
  error?: string;
  text_response?: string;
};

type RemoteTurn = {
  id?: string;
  prompt?: string;
  model?: ImageTurn["model"];
  mode?: ImageConversationMode;
  reference_images?: RemoteReferenceImage[];
  images?: RemoteResultImage[];
  count?: number;
  size?: string;
  quality?: ImageTurn["quality"];
  visibility?: ImageTurn["visibility"];
  status?: ImageTurnStatus;
  error?: string;
  created_at?: string;
  updated_at?: string;
};

type RemoteConversation = {
  id?: string;
  title?: string;
  created_at?: string;
  updated_at?: string;
  turns?: RemoteTurn[];
};

export type PromptVisibility = "private" | "public";

export type PromptLibraryItem = {
  id: string;
  owner_id: string;
  owner_name: string;
  title: string;
  body: string;
  tags: string[];
  category: string;
  note: string;
  model: string;
  use_case: string;
  visibility: PromptVisibility;
  source_conversation_id?: string;
  source_turn_id?: string;
  source_image_path?: string;
  created_at: string;
  updated_at: string;
};

export type PromptLibraryInput = {
  id?: string;
  title: string;
  body: string;
  tags?: string[];
  category?: string;
  note?: string;
  model?: string;
  use_case?: string;
  visibility?: PromptVisibility;
  source_conversation_id?: string;
  source_turn_id?: string;
  source_image_path?: string;
};

export type ConversationAttachment = {
  id: string;
  name: string;
  type: string;
  size: number;
  url: string;
  path: string;
};

function remoteReferenceImage(image: StoredReferenceImage): RemoteReferenceImage {
  return {
    id: image.id,
    name: image.name,
    type: image.type,
    url:
      image.url ||
      (image.dataUrl && !image.dataUrl.startsWith("data:") && !image.dataUrl.startsWith("blob:")
        ? image.dataUrl
        : ""),
    source: image.source,
  };
}

function referenceImagePersistentURL(image: StoredReferenceImage) {
  if (image.url) {
    return image.url;
  }
  if (image.dataUrl && !image.dataUrl.startsWith("data:") && !image.dataUrl.startsWith("blob:")) {
    return image.dataUrl;
  }
  return "";
}

function dataURLToReferenceFile(dataUrl: string, fileName: string, mimeType: string) {
  const [header, content] = dataUrl.split(",", 2);
  const matchedMimeType = header.match(/data:(.*?);base64/i)?.[1];
  const binary = atob(content || "");
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return new File([bytes], fileName, { type: mimeType || matchedMimeType || "image/png" });
}

async function previewToReferenceFile(image: StoredReferenceImage, index: number) {
  const fileName = image.name || `reference-${index + 1}.png`;
  const mimeType = image.type || "image/png";
  if (image.dataUrl?.startsWith("data:")) {
    return dataURLToReferenceFile(image.dataUrl, fileName, mimeType);
  }
  if (image.dataUrl?.startsWith("blob:")) {
    const response = await fetch(image.dataUrl);
    if (!response.ok) {
      throw new Error("读取参考图失败");
    }
    const blob = await response.blob();
    return new File([blob], fileName, { type: mimeType || blob.type || "image/png" });
  }
  return null;
}

async function persistReferenceImage(conversationId: string, image: StoredReferenceImage, index: number) {
  if (referenceImagePersistentURL(image)) {
    return image;
  }
  const file = await previewToReferenceFile(image, index);
  if (!file) {
    return image;
  }
  const attachment = await uploadConversationAttachment(conversationId, file);
  return {
    ...image,
    id: attachment.id,
    name: attachment.name || image.name || file.name,
    type: attachment.type || image.type || file.type || "image/png",
    url: attachment.url,
  };
}

async function conversationWithPersistentReferenceImages(conversation: ImageConversation) {
  const turns = await Promise.all(
    conversation.turns.map(async (turn) => ({
      ...turn,
      referenceImages: await Promise.all(
        turn.referenceImages.map((image, index) => persistReferenceImage(conversation.id, image, index)),
      ),
    })),
  );
  return { ...conversation, turns };
}

function remoteResultImage(image: StoredImage): RemoteResultImage {
  return {
    id: image.id,
    task_id: image.taskId,
    status: image.status,
    path: image.path,
    visibility: image.visibility,
    b64_json: image.b64_json,
    url: image.url,
    revised_prompt: image.revised_prompt,
    error: image.error,
    text_response: image.text_response,
  };
}

function remoteTurn(turn: ImageTurn): RemoteTurn {
  return {
    id: turn.id,
    prompt: turn.prompt,
    model: turn.model,
    mode: turn.mode,
    reference_images: turn.referenceImages.map(remoteReferenceImage),
    images: turn.images.map(remoteResultImage),
    count: turn.count,
    size: turn.size,
    quality: turn.quality,
    visibility: turn.visibility,
    status: turn.status,
    error: turn.error,
    created_at: turn.createdAt,
    updated_at: turn.createdAt,
  };
}

function remoteConversation(conversation: ImageConversation): RemoteConversation {
  return {
    id: conversation.id,
    title: conversation.title,
    created_at: conversation.createdAt,
    updated_at: conversation.updatedAt,
    turns: conversation.turns.map(remoteTurn),
  };
}

function localReferenceImage(image: RemoteReferenceImage): StoredReferenceImage {
  const url = image.url || "";
  return {
    id: image.id,
    name: image.name || "reference.png",
    type: image.type || "image/png",
    dataUrl: url,
    url,
    source: image.source === "upload" || image.source === "conversation" ? image.source : undefined,
  };
}

function localResultImage(image: RemoteResultImage): StoredImage {
  return {
    id: image.id || image.task_id || `${Date.now()}`,
    taskId: image.task_id,
    status: image.status,
    path: image.path,
    visibility: image.visibility,
    b64_json: image.b64_json,
    url: image.url,
    revised_prompt: image.revised_prompt,
    error: image.error,
    text_response: image.text_response,
  };
}

function localTurn(turn: RemoteTurn): ImageTurn {
  const createdAt = turn.created_at || new Date().toISOString();
  return {
    id: turn.id || `${Date.now()}`,
    prompt: turn.prompt || "",
    model: turn.model || "auto",
    mode: turn.mode || "generate",
    referenceImages: Array.isArray(turn.reference_images) ? turn.reference_images.map(localReferenceImage) : [],
    count: Math.max(1, Number(turn.count || 1)),
    size: turn.size || "",
    quality: turn.quality,
    visibility: turn.visibility,
    images: Array.isArray(turn.images) ? turn.images.map(localResultImage) : [],
    createdAt,
    status: turn.status || "success",
    error: turn.error,
  };
}

function localConversation(conversation: RemoteConversation): ImageConversation {
  const createdAt = conversation.created_at || new Date().toISOString();
  return {
    id: conversation.id || `${Date.now()}`,
    title: conversation.title || "",
    createdAt,
    updatedAt: conversation.updated_at || createdAt,
    turns: Array.isArray(conversation.turns) ? conversation.turns.map(localTurn) : [],
  };
}

export async function fetchImageConversations() {
  const data = await httpRequest<{ items?: RemoteConversation[] | null }>("/api/image-conversations");
  return Array.isArray(data.items) ? data.items.map(localConversation) : [];
}

export async function fetchImageConversation(id: string) {
  const data = await httpRequest<{ item: RemoteConversation }>(`/api/image-conversations/${encodeURIComponent(id)}`);
  return localConversation(data.item);
}

export async function saveImageConversationRemote(conversation: ImageConversation) {
  const persistentConversation = await conversationWithPersistentReferenceImages(conversation);
  const data = await httpRequest<{ item: RemoteConversation }>("/api/image-conversations", {
    method: "POST",
    body: remoteConversation(persistentConversation),
  });
  return localConversation(data.item);
}

export async function deleteImageConversationRemote(id: string) {
  await httpRequest<{ ok: boolean }>(`/api/image-conversations/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function fetchConversationAttachmentBlobURL(url: string) {
  return URL.createObjectURL(await fetchConversationAttachmentBlob(url));
}

export async function fetchConversationAttachmentFile(url: string, fileName: string, mimeType?: string) {
  const blob = await fetchConversationAttachmentBlob(url);
  return new File([blob], fileName, { type: mimeType || blob.type || "image/png" });
}

async function fetchConversationAttachmentBlob(url: string) {
  const token = await getStoredSessionToken();
  const baseURL = webConfig.apiUrl.replace(/\/$/, "");
  const response = await fetch(url.startsWith("http") ? url : `${baseURL}${url}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
  });
  if (!response.ok) {
    throw new Error("读取参考图失败");
  }
  return response.blob();
}

export async function uploadConversationAttachment(conversationId: string, file: File) {
  const formData = new FormData();
  formData.append("file", file);
  const data = await httpRequest<{ item: ConversationAttachment }>(
    `/api/image-conversations/${encodeURIComponent(conversationId)}/attachments`,
    {
      method: "POST",
      body: formData,
    },
  );
  return data.item;
}

export async function fetchPrompts(filters: { scope?: "visible" | "all" | "mine" | "public"; q?: string; category?: string; tag?: string } = {}) {
  const params = new URLSearchParams();
  if (filters.scope) params.set("scope", filters.scope);
  if (filters.q) params.set("q", filters.q);
  if (filters.category) params.set("category", filters.category);
  if (filters.tag) params.set("tag", filters.tag);
  const data = await httpRequest<{ items?: PromptLibraryItem[] | null }>(
    `/api/prompts${params.toString() ? `?${params.toString()}` : ""}`,
  );
  return Array.isArray(data.items) ? data.items : [];
}

export async function savePrompt(input: PromptLibraryInput) {
  const data = await httpRequest<{ item: PromptLibraryItem }>(
    input.id ? `/api/prompts/${encodeURIComponent(input.id)}` : "/api/prompts",
    {
      method: input.id ? "PATCH" : "POST",
      body: input,
    },
  );
  return data.item;
}

export async function deletePrompt(id: string) {
  await httpRequest<{ ok: boolean }>(`/api/prompts/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function copyPrompt(id: string) {
  const data = await httpRequest<{ item: PromptLibraryItem }>(`/api/prompts/${encodeURIComponent(id)}/copy`, {
    method: "POST",
  });
  return data.item;
}

export async function updateImagePromptMetadata(input: {
  path: string;
  manual_prompt?: string;
  conversation_id?: string;
  turn_id?: string;
  task_id?: string;
  prompt_id?: string;
}) {
  return httpRequest<{ item: Partial<ManagedImage> & { path: string } }>("/api/images/prompt-metadata", {
    method: "PATCH",
    body: input,
  });
}
