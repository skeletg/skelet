// PUTs a file to a presigned storage URL - the step every upload flow shares between its request and confirm
// mutations. `what` names the upload in the error, "Banner upload failed with HTTP 403."
export async function putToSignedUrl(url: string, file: File, what = 'Upload'): Promise<void> {
  const res = await fetch(url, { method: 'PUT', headers: { 'Content-Type': file.type }, body: file })
  if (!res.ok) throw new Error(`${what} failed with HTTP ${res.status}.`)
}
