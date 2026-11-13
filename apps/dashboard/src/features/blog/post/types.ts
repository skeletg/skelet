import type { useRecordForm } from '@/shared/saving/useRecordForm'
import type { BlogPostQuery } from '../Blog.gql'
import type { PostValues } from '../schemas/post'

export type Post = NonNullable<BlogPostQuery['blogPost']>
export type PostFormApi = ReturnType<typeof useRecordForm<PostValues>>['form']
