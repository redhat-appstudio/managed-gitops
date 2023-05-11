package rhtappromotion

import (
	appstudiosharedv1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/managed-gitops/tests-e2e/fixture"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func BuildPromotionRunResource(name, appName, snapshotName, targetEnvironment string) appstudiosharedv1.PromotionRun {

	promotionRun := appstudiosharedv1.PromotionRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fixture.GitOpsServiceE2ENamespace,
		},
		Spec: appstudiosharedv1.PromotionRunSpec{
			Snapshot:    snapshotName,
			Application: appName,
			ManualPromotion: appstudiosharedv1.ManualPromotionConfiguration{
				TargetEnvironment: targetEnvironment,
			},
		},
	}
	return promotionRun
}
