package hotfix

import (
	"context"
	"fmt"
	"strings"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/db"
)

// HotfixK8sResourceUIDOfKubernetesResourceToDBResourceMapping will update a KubernetesToDBResourceMapping row
// that matches as follows.
//
// If the following values match exactly:
// - KubernetesResourceType
// - KubernetesResourceUID
// - DBRelationType
// - DBRelationKey
//
// then update this value to a new value:
// - KubernetesResourceUID

func HotfixK8sResourceUIDOfKubernetesResourceToDBResourceMapping(ctx context.Context, kdbToPatch db.KubernetesToDBResourceMapping,
	oldK8sResourceUID string, newK8sResourceUID string) error {

	dbq, err := db.NewSharedProductionPostgresDBQueries(false)
	if err != nil {
		return fmt.Errorf("unable to acquire database in HotfixK8sResourceUIDOfKubernetesResourceToDBResourceMapping function, %v", err)
	}

	targetKDB := kdbToPatch

	if err := dbq.GetKubernetesResourceMappingForDatabaseResource(ctx, &targetKDB); err != nil {
		if db.IsResultNotFoundError(err) {
			fmt.Println("Target KubernetesToDBResourceMapping does not exist in database, no patch was needed.")
			return nil
		}

		// If the database has not yet been initialized, this error will be returned:
		// - ERROR #42P01 relation "kubernetestodbresourcemapping" does not exist
		// In this case, since there is no database to patch, no work is needed, and no error is returned.
		// The database will be initialized by the controller once the init-container has completed.
		if strings.Contains(err.Error(), "ERROR #42P01") {
			return nil
		}

		return fmt.Errorf("unable to retrieve patched KubernetesDBToResourceMapping, %v", err)
	}

	if targetKDB.DBRelationKey != kdbToPatch.DBRelationKey {
		fmt.Println("DBRelationKey did not match: patch was not needed")
		return nil
	}

	if targetKDB.DBRelationType != kdbToPatch.DBRelationType {
		fmt.Println("DBRelationType did not match: patch was not needed")
		return nil
	}

	if targetKDB.KubernetesResourceType != kdbToPatch.KubernetesResourceType {
		fmt.Println("KubernetesResourceType did not match: patch was not needed")
		return nil
	}

	if targetKDB.KubernetesResourceUID != oldK8sResourceUID {
		fmt.Println("kubernetesDBToResourceMapping patch was not needed")
		return nil
	}

	fmt.Println("Patch is required, calling UpdateKubernetesResourceUIDForKubernetesToDBResourceMapping", oldK8sResourceUID, newK8sResourceUID)
	targetKDB.KubernetesResourceUID = newK8sResourceUID
	if err := dbq.UpdateKubernetesResourceUIDForKubernetesToDBResourceMapping(ctx, &targetKDB); err != nil {
		return fmt.Errorf("unable to patch KubernetesDBToResourceMapping: %v", err)
	}

	return nil
}
