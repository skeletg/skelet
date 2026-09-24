import { z } from 'zod'

// New blog post - title, slug and first locale; body, status, categories and tags come on the post's
// own page. The slug is permanent in URLs, though a change later leaves a 301 behind

export const newPostSchema = z.object({
  title: z.string().trim().min(1, 'Title is required'),
  slug: z.string().trim().min(1, 'Slug is required'),
  locale: z.string().min(1, 'Pick the initial locale'),
  excerpt: z.string(),
})

export type NewPostValues = z.infer<typeof newPostSchema>

export function toNewPostInput(v: NewPostValues) {
  const excerpt = v.excerpt.trim()
  return {
    slug: v.slug.trim(),
    locale: v.locale,
    title: v.title.trim(),
    // Stored as rich text - a plain excerpt becomes one paragraph
    excerpt: excerpt ? { root: { children: [{ type: 'paragraph', children: [{ type: 'text', text: excerpt }] }] } } : null,
  }
}
