// Package patches is the home for core, non-plugin data patches - each lives in its own file and self-registers from init() with datapatch.Register - The CLI blank-imports this package so those run, and plugin patches register the same way through the SDK
//
// Empty on purpose: reference data belongs in the reference-data migration, and a backfill only earns a patch when it needs Go the schema can't express - walking a block tree, encrypting with a key from the environment, anything reading a code-defined registry
package patches
