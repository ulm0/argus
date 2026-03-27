//go:build !arm

package updater

// goarm is unused on non-ARM platforms but satisfies the reference in assetNameForArch.
const goarm = ""
