// Registered via vitest.config.ts setupFiles - testing-library only installs its
// own afterEach(cleanup) when vitest runs with globals:true, which this repo
// doesn't, so without this every render in a test file stacks into one document
// and multi-render files trip over duplicate elements
import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

afterEach(cleanup)
