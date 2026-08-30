// Barrel for core Lexical blocks - importing it fires each block's registerLexicalBlock, and plugins register against the same registry via loader.ts, so both sets are live before the editor mounts

import './product-image'
import './faq'
import './structured'

export {
  getLexicalBlockNodes,
  getLexicalBlockToolbarEntries,
  extractPlainTextFor,
  registerLexicalBlock,
  type LexicalBlockDefinition,
  type LexicalBlockToolbarEntry,
} from './registry'

export { $createProductImageNode, ProductImageNode } from './product-image'
