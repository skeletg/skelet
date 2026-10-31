// ProductImage block - figure plus optional caption, inserted from the editor's image-upload
// toolbar button rather than the "Custom Blocks" menu, so this registers a node but no entry
//
// SYNC: SerializedProductImageNode is mirrored in
// templates/vetrina/app/lib/lexical-render.tsx - renaming a field here without
// updating the channel silently breaks blog post rendering

import { DecoratorNode, type NodeKey } from 'lexical'
import type { ReactNode } from 'react'

import { registerLexicalBlock } from './registry'

type SerializedProductImageNode = {
  altText: string
  height: number
  src: string
  type: 'product-image'
  version: 1
  width: number
}

export class ProductImageNode extends DecoratorNode<ReactNode> {
  __src: string
  __altText: string
  __width: number
  __height: number

  static getType(): string {
    return 'product-image'
  }

  static clone(node: ProductImageNode): ProductImageNode {
    return new ProductImageNode(node.__src, node.__altText, node.__width, node.__height, node.__key)
  }

  static importJSON(serializedNode: SerializedProductImageNode): ProductImageNode {
    return new ProductImageNode(
      serializedNode.src,
      serializedNode.altText,
      serializedNode.width,
      serializedNode.height,
    )
  }

  constructor(src: string, altText: string, width: number, height: number, key?: NodeKey) {
    super(key)
    this.__src = src
    this.__altText = altText
    this.__width = width
    this.__height = height
  }

  exportJSON(): SerializedProductImageNode {
    return {
      altText: this.__altText,
      height: this.__height,
      src: this.__src,
      type: 'product-image',
      version: 1,
      width: this.__width,
    }
  }

  // A figure with caption shouldn't sit inside a <p> - without these, $insertNodes from a range
  // selection nests the image in the current paragraph, which is invalid and harder to render
  isInline(): false {
    return false
  }

  isTopLevel(): true {
    return true
  }

  createDOM(): HTMLElement {
    const element = document.createElement('div')
    element.className = 'my-3'
    return element
  }

  updateDOM(): false {
    return false
  }

  decorate(): ReactNode {
    return (
      <figure className="my-3 overflow-hidden rounded-md border bg-muted/20">
        <img
          src={this.__src}
          alt={this.__altText}
          width={this.__width}
          height={this.__height}
          className="h-auto max-w-full"
        />
        {this.__altText ? (
          <figcaption className="border-t px-3 py-2 text-xs text-muted-foreground">
            {this.__altText}
          </figcaption>
        ) : null}
      </figure>
    )
  }
}

export function $createProductImageNode(src: string, altText: string, width: number, height: number): ProductImageNode {
  return new ProductImageNode(src, altText, width, height)
}

registerLexicalBlock({
  node: ProductImageNode,
})
