import { gql } from '@apollo/client'

export type MediaAssetStatus = 'PENDING' | 'READY'
export type MediaMimeGroup = 'IMAGE' | 'DOCUMENT'

export type MediaAsset = {
  __typename?: 'MediaAsset'
  id: string
  path: string
  directoryPath: string
  filename: string
  url: string
  mimeType: string
  mimeGroup: MediaMimeGroup
  sizeBytes: number
  width?: number | null
  height?: number | null
  status: MediaAssetStatus
  title: string
  altText: string
  caption: string
  notes: string
  focalX?: number | null
  focalY?: number | null
  createdAt: string
  updatedAt: string
}

export type MediaDirectoryChild = {
  __typename?: 'MediaDirectoryChild'
  path: string
  name: string
}

export type UntrackedMediaFile = {
  __typename?: 'UntrackedMediaFile'
  objectKey: string
  url: string
  filename: string
  mimeType: string
  sizeBytes: number
  lastModified: string
}

export type MediaDirectoryQuery = {
  mediaDirectory: {
    __typename?: 'MediaDirectory'
    path: string
    name: string
    directories: MediaDirectoryChild[]
    assets: MediaAsset[]
    untrackedFiles: UntrackedMediaFile[]
  }
}

export type MediaDirectoryQueryVariables = {
  path: string
}

export type SearchMediaAssetsQuery = {
  searchMediaAssets: {
    __typename?: 'MediaAssetSearchResult'
    totalCount: number
    items: MediaAsset[]
  }
}

export type SearchMediaAssetsQueryVariables = {
  query?: string | null
  path?: string | null
  mimeGroup?: MediaMimeGroup | null
  limit?: number | null
  offset?: number | null
}

export type MediaAssetByIdQuery = { mediaAsset: MediaAsset }
export type MediaAssetByIdQueryVariables = { id: string }

export type RequestMediaAssetUploadInput = {
  path: string
  filename: string
  mimeType: string
  sizeBytes: number
  title?: string | null
  altText?: string | null
  caption?: string | null
  notes?: string | null
}

export type RequestMediaAssetUploadMutation = {
  requestMediaAssetUpload: {
    __typename?: 'RequestMediaAssetUploadPayload'
    uploadUrl: string
    expiresAt: string
    asset: MediaAsset
  }
}

export type RequestMediaAssetUploadMutationVariables = {
  input: RequestMediaAssetUploadInput
}

export type ConfirmMediaAssetUploadMutation = {
  confirmMediaAssetUpload: MediaAsset
}

export type ConfirmMediaAssetUploadMutationVariables = {
  assetId: string
  width?: number | null
  height?: number | null
}

export type UpdateMediaAssetMutation = {
  updateMediaAsset: MediaAsset
}

export type SetMediaFocalPointMutation = {
  setMediaFocalPoint: MediaAsset
}

export type SetMediaFocalPointMutationVariables = {
  assetId: string
  x?: number | null
  y?: number | null
}

export type UpdateMediaAssetMutationVariables = {
  input: {
    id: string
    title?: string | null
    altText?: string | null
    caption?: string | null
    notes?: string | null
  }
}

export type MoveMediaAssetMutation = {
  moveMediaAsset: MediaAsset
}

export type MoveMediaAssetMutationVariables = {
  id: string
  targetPath: string
  filename?: string | null
}

export type DeleteMediaAssetMutation = {
  deleteMediaAsset: boolean
}

export type DeleteMediaAssetMutationVariables = {
  id: string
  deleteFromStorage?: boolean | null
}

export type CreateMediaDirectoryMutation = {
  createMediaDirectory: MediaDirectoryChild
}

export type CreateMediaDirectoryMutationVariables = {
  path: string
  name: string
}

export type RenameMediaDirectoryMutation = {
  renameMediaDirectory: MediaDirectoryChild
}

export type RenameMediaDirectoryMutationVariables = {
  path: string
  newName: string
}

export type DeleteMediaDirectoryMutation = {
  deleteMediaDirectory: boolean
}

export type DeleteMediaDirectoryMutationVariables = {
  path: string
  recursive: boolean
}

const MediaAssetFields = gql`
  fragment MediaAssetFields on MediaAsset {
    id
    path
    directoryPath
    filename
    url
    mimeType
    mimeGroup
    sizeBytes
    width
    height
    status
    title
    altText
    caption
    notes
    focalX
    focalY
    createdAt
    updatedAt
  }
`

export const MediaDirectoryDocument = gql`
  ${MediaAssetFields}
  query MediaDirectory($path: String!) {
    mediaDirectory(path: $path) {
      path
      name
      directories {
        path
        name
      }
      assets {
        ...MediaAssetFields
      }
      untrackedFiles {
        objectKey
        url
        filename
        mimeType
        sizeBytes
        lastModified
      }
    }
  }
`

export const SearchMediaAssetsDocument = gql`
  ${MediaAssetFields}
  query SearchMediaAssets($query: String, $path: String, $mimeGroup: MediaMimeGroup, $limit: Int, $offset: Int) {
    searchMediaAssets(query: $query, path: $path, mimeGroup: $mimeGroup, limit: $limit, offset: $offset) {
      totalCount
      items {
        ...MediaAssetFields
      }
    }
  }
`

export type AllMediaDirectoriesQuery = {
  allMediaDirectories: { path: string; name: string }[]
}

export const MediaAssetByIdDocument = gql`
  ${MediaAssetFields}
  query MediaAssetById($id: ID!) {
    mediaAsset(id: $id) {
      ...MediaAssetFields
    }
  }
`

export const AllMediaDirectoriesDocument = gql`
  query AllMediaDirectories {
    allMediaDirectories {
      path
      name
    }
  }
`

export const CreateMediaDirectoryDocument = gql`
  mutation CreateMediaDirectory($path: String!, $name: String!) {
    createMediaDirectory(path: $path, name: $name) {
      path
      name
    }
  }
`

export const RequestMediaAssetUploadDocument = gql`
  ${MediaAssetFields}
  mutation RequestMediaAssetUpload($input: RequestMediaAssetUploadInput!) {
    requestMediaAssetUpload(input: $input) {
      uploadUrl
      expiresAt
      asset {
        ...MediaAssetFields
      }
    }
  }
`

export const ConfirmMediaAssetUploadDocument = gql`
  ${MediaAssetFields}
  mutation ConfirmMediaAssetUpload($assetId: ID!, $width: Int, $height: Int) {
    confirmMediaAssetUpload(assetId: $assetId, width: $width, height: $height) {
      ...MediaAssetFields
    }
  }
`

export const UpdateMediaAssetDocument = gql`
  ${MediaAssetFields}
  mutation UpdateMediaAsset($input: UpdateMediaAssetInput!) {
    updateMediaAsset(input: $input) {
      ...MediaAssetFields
    }
  }
`

export const SetMediaFocalPointDocument = gql`
  ${MediaAssetFields}
  mutation SetMediaFocalPoint($assetId: ID!, $x: Float, $y: Float) {
    setMediaFocalPoint(assetId: $assetId, x: $x, y: $y) {
      ...MediaAssetFields
    }
  }
`

// AI assistant (hand-authored - media docs aren't codegen). Named MediaAiStatus
// to avoid an operation-name collision with the CMS AiStatus query.
export type MediaAiStatusQuery = { aiStatus: { __typename?: 'AIStatus'; enabled: boolean } }
export const MediaAiStatusDocument = gql`
  query MediaAiStatus {
    aiStatus {
      enabled
    }
  }
`

export type GenerateMediaAltTextMutation = { generateMediaAltText: string }
export type GenerateMediaAltTextMutationVariables = { assetId: string }
export const GenerateMediaAltTextDocument = gql`
  mutation GenerateMediaAltText($assetId: ID!) {
    generateMediaAltText(assetId: $assetId)
  }
`

export const MoveMediaAssetDocument = gql`
  ${MediaAssetFields}
  mutation MoveMediaAsset($id: ID!, $targetPath: String!, $filename: String) {
    moveMediaAsset(id: $id, targetPath: $targetPath, filename: $filename) {
      ...MediaAssetFields
    }
  }
`

export const DeleteMediaAssetDocument = gql`
  mutation DeleteMediaAsset($id: ID!, $deleteFromStorage: Boolean) {
    deleteMediaAsset(id: $id, deleteFromStorage: $deleteFromStorage)
  }
`

export const RenameMediaDirectoryDocument = gql`
  mutation RenameMediaDirectory($path: String!, $newName: String!) {
    renameMediaDirectory(path: $path, newName: $newName) {
      path
      name
    }
  }
`

export const DeleteMediaDirectoryDocument = gql`
  mutation DeleteMediaDirectory($path: String!, $recursive: Boolean!) {
    deleteMediaDirectory(path: $path, recursive: $recursive)
  }
`
