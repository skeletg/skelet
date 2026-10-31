"use server";

// Custom form (`form` CMS block) submission + presigned file uploads
// File bytes go browser -> object storage directly, never proxied through Next

import { gqlFetch, GraphQLRequestError } from "@/app/lib/graphql";
import { PresignWebFormUploadDocument, SubmitWebFormDocument } from "@/app/lib/webforms/operations.gql";

export type WebFormFile = {
  fieldKey: string;
  objectKey: string;
  name: string;
};

export type WebFormSubmitResult = {
  ok: boolean;
  successMessage?: string | null;
  redirectUrl?: string | null;
  errors: { field: string; message: string }[];
};

export async function submitWebFormAction(args: {
  formId: string;
  values: Record<string, unknown>;
  files?: WebFormFile[];
  honeypot?: string;
  captchaToken?: string;
}): Promise<WebFormSubmitResult> {
  try {
    const res = await gqlFetch(SubmitWebFormDocument, {
      input: {
        formId: args.formId,
        values: args.values,
        files: args.files ?? [],
        honeypot: args.honeypot ?? "",
        captchaToken: args.captchaToken ?? "",
      },
    });
    const r = res.data.submitWebForm;
    return {
      ok: r.ok,
      successMessage: r.successMessage,
      redirectUrl: r.redirectUrl,
      errors: r.errors ?? [],
    };
  } catch (err) {
    const message =
      err instanceof GraphQLRequestError ? err.message : "Something went wrong. Please try again.";
    return { ok: false, errors: [{ field: "", message }] };
  }
}

export async function presignWebFormUploadAction(args: {
  formId: string;
  fieldKey: string;
  filename: string;
  contentType: string;
  sizeBytes: number;
}): Promise<{ url: string; objectKey: string } | { error: string }> {
  try {
    const res = await gqlFetch(PresignWebFormUploadDocument, args);
    return { url: res.data.presignWebFormUpload.url, objectKey: res.data.presignWebFormUpload.objectKey };
  } catch (err) {
    const message =
      err instanceof GraphQLRequestError ? err.message : "Upload could not be prepared.";
    return { error: message };
  }
}
