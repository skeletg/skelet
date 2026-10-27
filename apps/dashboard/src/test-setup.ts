// Registered via vitest.config.ts setupFiles - testing-library only installs its
// own afterEach(cleanup) when vitest runs with globals:true, which this repo
// doesn't, so without this every render in a test file stacks into one document
// and multi-render files trip over duplicate elements
import { afterEach } from 'vitest'
import { cleanup, configure } from '@testing-library/react'
import { MockLink } from '@apollo/client/testing'

afterEach(cleanup)

// Apollo 4 delays every mocked response by a random 20-50ms; a test that ends once the request is
// sent then leaves the save's setState landing after jsdom is torn down ("window is not defined")
MockLink.defaultOptions = { delay: 0 }

// A page's first render waits on several mocked queries; with the whole suite loading the machine
// that outruns testing-library's 1s default for findBy/waitFor, and the page tests flake
configure({ asyncUtilTimeout: 3000 })
