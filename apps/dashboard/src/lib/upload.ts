// PUTs a file to a presigned storage URL - the step every upload flow shares between its request and confirm
// mutations. `what` names the upload in the error, "Banner upload failed with HTTP 403."
export async function putToSignedUrl(url: string, file: File, what = 'Upload'): Promise<void> {
  const res = await fetch(url, { method: 'PUT', headers: { 'Content-Type': file.type }, body: file })
  if (!res.ok) throw new Error(`${what} failed with HTTP ${res.status}.`)
}

// The same PUT, reporting how much has gone up as a 0..1 fraction - fetch can't watch an upload's body, XHR can
export function putToSignedUrlWithProgress(url: string, file: File, onProgress: (fraction: number) => void, what = 'Upload'): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('PUT', url)
    xhr.setRequestHeader('Content-Type', file.type)
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(e.loaded / e.total)
    }
    xhr.onload = () => (xhr.status >= 200 && xhr.status < 300 ? resolve() : reject(new Error(`${what} failed with HTTP ${xhr.status}.`)))
    xhr.onerror = () => reject(new Error(`${what} failed - the connection dropped.`))
    xhr.send(file)
  })
}
