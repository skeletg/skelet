// Package patches is the home for core, non-plugin data patches - each lives in its own file and self-registers from init() with datapatch.Register
// The CLI blank-imports this package so those run, and plugin patches register the same way through the SDK
package patches
