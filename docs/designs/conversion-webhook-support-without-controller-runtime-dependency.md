# Support for Conversion Webhooks in application-api without adding controller-runtime dependency

### Written by
- Jonathan West (@jgwest)
- Originally written November 7th, 2023


# Introduction

As part of [GITOPSRVCE-664](https://issues.redhat.com/browse/GITOPSRVCE-664) we are trying to do the following:

* Define a new AppStudio Environment version, [v1beta1](https://github.com/jparsai/application-api/blob/8d5fe3ea2875685a14edb101fe78e9c0777e9c6d/api/v1beta1/environment_types.go#L24)  
* Create a conversion webhook to automatically migrate between [v1alpha1](https://github.com/redhat-appstudio/application-api/blob/89515ad2504f1f60b398d640ec8dac537133b1a2/api/v1alpha1/environment_types.go#L24) and [v1beta1](https://github.com/jparsai/application-api/blob/8d5fe3ea2875685a14edb101fe78e9c0777e9c6d/api/v1beta1/environment_types.go#L24) of the AppStudio Environment API (via controller runtime)  
  * This is the standard mechanism for maintaining multiple versions of a K8s API resource.  
  * For reference, for those unfamiliar with conversion webhooks, [here is an example of an existing AppStudio conversion webhook](https://github.com/redhat-appstudio/integration-service/blob/82323e9e4dfca1dde1073d3a3d9a67e5ee2aa28a/api/v1alpha1/integrationtestscenario_conversion.go#L36) from the integration service component. (Note: this is just an example and is not affected by the issues described in this document.)

However, it appears that recent changes to the application-api repo – removing controller-runtime as a dependency – prevent us from implementing conversion webhooks for any of APIs defined within that repository.

# Description

**A)** To implement a conversion webhook, the **convertObject** [\[1\]](https://github.com/kubernetes-sigs/controller-runtime/blob/c30c66d67f47feeaf2cf0816e11c6ec0260c6e55/pkg/webhook/conversion/conversion.go#L123) in **pkg/webhook/conversion/conversion.go** of controller-runtime requires K8s API objects to implement the **Convertible** and **Hub** interfaces[\[2\]](https://github.com/kubernetes-sigs/controller-runtime/blob/c30c66d67f47feeaf2cf0816e11c6ec0260c6e55/pkg/conversion/conversion.go#L27). All conversion webhooks (implemented using controller-runtime), used for conversion between K8s API Resource versions, must implement these interfaces.

**B)** In this case (via A) ), the v1alpha1 and v1beta1 versions of the AppStudio **Environment** object must define Hub(), ConvertTo(), and ConvertFrom() functions, otherwise Environment will not be convertible via controller-runtime's conversion webhook logic.  
   	   
**C)** However, It is not possible to add these functions to Environment object, from outside the application-api module, as Go prevents this:

```go
// when this function is defined within the managed-gitops repo under the appstudio-controller module (e.g. outside application-api) it produces this error:
// 	cannot define new methods on non-local type "github.com/redhat-appstudio/application-api/api/v1beta1".Environment    

func (src *appstudiov1beta1.Environment) ConvertTo(dstRaw conversion.Hub) error {
}
```

(Go requires functions on structs to be local to the package where the struct is defined)

**D)** Thus, the Hub, ConvertTo, and ConvertFrom functions must be defined in the application-api module on the Environment object(s). (via **A**, **B**, **C**)

**E)** Since, the Hub, ConvertTo, and ConvertFrom functions [directly reference](https://github.com/kubernetes-sigs/controller-runtime/blob/c30c66d67f47feeaf2cf0816e11c6ec0260c6e55/pkg/conversion/conversion.go#L29C18-L29C18) controller-runtime API interfaces, we cannot add those functions to Environment as-is. Doing so would add a dependency to controller-runtime from application-api, which is what we want to avoid in the first place.

**F**) We likewise cannot implement our own interfaces that have the same shape as controller-runtime’s Hub and Convertible interfaces:

```go
// What is defined below produces the following error:
//
// Cannot use &Environment{} (value of type *Environment) as "sigs.k8s.io/controller-runtime/pkg/conversion".Convertible value in variable declaration: *Environment does not implement "sigs.k8s.io/controller-runtime/pkg/conversion".Convertible (wrong type for method ConvertFrom)
//   	 have ConvertFrom(MyHub) error
//   	 want ConvertFrom("sigs.k8s.io/controller-runtime/pkg/conversion".Hub) error
//
// Environment must implement Convertible by A), but doing so requires using controller-runtime's Hub interface, rather than an interface with the same shape.

// Convertible defines capability of a type to convertible i.e. it can be converted to/from a hub type.
type MyConvertible interface {
    runtime.Object
    ConvertTo(dst MyHub) error
    ConvertFrom(src MyHub) error
}

// Hub marks that a given type is the hub type for conversion. This means that
// all conversions will first convert to the hub type, then convert from the hub
// type to the destination type. All types besides the hub type should implement
// Convertible.
type MyHub interface {
    runtime.Object
    Hub()
}
var _ conversion.Convertible = &Environment{} 
```

**G)** There exist no other mechanisms to add a Go function to a Go Object (i.e. implement an interface on an Object) within a package besides **E)** and **F)**.

**H)** Since:

* the Environment object must implement the Hub, ConvertTo, and ConvertFrom functions in application-api (via **D**)  
* and we cannot add those to the Environment API in application-api as-is due to controller-runtime dependency (via **E**)  
* and we cannot add those to application-api via a new interface with the same shape (via **F**)  
* and there exist no other options for implementing an interface on an object (via **G**)

Then it is not possible to implement a conversion webhook without adding a dependency (back) from application-api to controller-runtime.

# Ramifications

Presuming the above is true, that leaves us with 2 options:

## Option A \- we can’t use conversion webhooks on application-api API CRs (such as Application, Component, Environment, etc): instead make gradual breaking changes on a single API version

Without the ability to support conversion webhooks on application-api resources, any fields we want to deprecate must follow this process. 

* For a new **fieldB**, and a deprecated **fieldA**  
1) Add a new field **fieldB** to the API CR  
2) Update all existing controllers to reconcile both the old field **fieldA** and the new field **fieldB**, simultaneously (e.g. if new **fieldB** exists, then use it, otherwise use old **fieldA**)  
3) Ensure that all controllers have moved to reconciling the new field **fieldB**  
4) Remove the old field, **fieldA**, from the API

This is significantly more work than the traditional K8s multi-version conversion webhook approach, which handles automated conversion between K8s versions.

## Option B \- revert back to requiring controller-runtime on application-api

If we revert back to requiring a specific version of controller-runtime on application-api (with all the baggage this comes with), this will allow us to use conversion webhooks on these CR APIs.

## Option C \- Mutating webhook that changes v1alpha1 Environments to v1beta1 Environments

- Webhook checks if the resource was a v1alpha1 type  
- If so, the webhook would create a new v1alpha2 object, perform the necessary steps to convert the object from v1alpha1 to v1alpha2. Then assign the runtime.Object object to the converted v1alpha2 object  
- It would look something like this
```go
func (r *EnvironmentWebhook) Default(ctx context.Context, obj runtime.Object) error {
   // TODO(user): fill in your defaulting logic.


   if obj.GetObjectKind().GroupVersionKind().Version == "v1alpha1" {
       env := obj.(*appstudiov1alpha1.Environment)


       newEnv := appstudiov1alpha2.Environment{}
       newEnv.ObjectMeta = env.ObjectMeta
       
      // Fill in conversion logic here


       obj = &newApp
   }


   return nil
}
```


## Option D \- A special controller to do the conversion from v1alpha1 to v1alpha2

- If it detects a v1alpha1 Environment resource is created, it deletes it from Kube, and creates a new v1alpha2 resource with the same namespaced name and has the converted spec  
- **Caveat**: Any other controllers that watch Environment resources would need to be updated to watch v1beta1 before doing this  
