package store

import "fmt"

// On-disk format versions. Writers use the *Write constants. Readers accept
// [Min, Write] inclusive; older docs are migrated in memory when needed.
const (
	storeWriteVersion = 3
	// storeMinVersion is the oldest meta.yaml version this binary can read. When it is below
	// storeWriteVersion, loadMetaAndKey rewrites meta.yaml to storeWriteVersion after unlock.
	storeMinVersion = 2

	userDocWriteVersion = 1
	userDocMinVersion   = 1

	hostDocWriteVersion = 1
	hostDocMinVersion   = 1
)

// docVersion is the user/host YAML version written by this binary (kept for callers/tests).
const docVersion = userDocWriteVersion

func validateReadableStoreVersion(v int) error {
	if v < storeMinVersion || v > storeWriteVersion {
		return fmt.Errorf("unsupported store version: %d (supported %d-%d)", v, storeMinVersion, storeWriteVersion)
	}
	return nil
}

func validateReadableUserDocVersion(alias string, v int) error {
	if v < userDocMinVersion || v > userDocWriteVersion {
		return fmt.Errorf("unsupported user doc version for %s: %d (supported %d-%d)", alias, v, userDocMinVersion, userDocWriteVersion)
	}
	return nil
}

func validateReadableHostDocVersion(alias string, v int) error {
	if v < hostDocMinVersion || v > hostDocWriteVersion {
		return fmt.Errorf("unsupported host doc version for %s: %d (supported %d-%d)", alias, v, hostDocMinVersion, hostDocWriteVersion)
	}
	return nil
}
