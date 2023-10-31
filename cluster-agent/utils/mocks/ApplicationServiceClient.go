// Code generated by mockery v2.36.0. DO NOT EDIT.

package mocks

import (
	application "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	apiclient "github.com/argoproj/argo-cd/v2/reposerver/apiclient"

	context "context"

	grpc "google.golang.org/grpc"

	mock "github.com/stretchr/testify/mock"

	v1 "k8s.io/api/core/v1"

	v1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

// ApplicationServiceClient is an autogenerated mock type for the ApplicationServiceClient type
type ApplicationServiceClient struct {
	mock.Mock
}

// Create provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) Create(ctx context.Context, in *application.ApplicationCreateRequest, opts ...grpc.CallOption) (*v1alpha1.Application, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *v1alpha1.Application
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationCreateRequest, ...grpc.CallOption) (*v1alpha1.Application, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationCreateRequest, ...grpc.CallOption) *v1alpha1.Application); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1alpha1.Application)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationCreateRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Delete provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) Delete(ctx context.Context, in *application.ApplicationDeleteRequest, opts ...grpc.CallOption) (*application.ApplicationResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *application.ApplicationResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationDeleteRequest, ...grpc.CallOption) (*application.ApplicationResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationDeleteRequest, ...grpc.CallOption) *application.ApplicationResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*application.ApplicationResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationDeleteRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeleteResource provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) DeleteResource(ctx context.Context, in *application.ApplicationResourceDeleteRequest, opts ...grpc.CallOption) (*application.ApplicationResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *application.ApplicationResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationResourceDeleteRequest, ...grpc.CallOption) (*application.ApplicationResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationResourceDeleteRequest, ...grpc.CallOption) *application.ApplicationResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*application.ApplicationResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationResourceDeleteRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Get provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) Get(ctx context.Context, in *application.ApplicationQuery, opts ...grpc.CallOption) (*v1alpha1.Application, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *v1alpha1.Application
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationQuery, ...grpc.CallOption) (*v1alpha1.Application, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationQuery, ...grpc.CallOption) *v1alpha1.Application); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1alpha1.Application)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationQuery, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetApplicationSyncWindows provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) GetApplicationSyncWindows(ctx context.Context, in *application.ApplicationSyncWindowsQuery, opts ...grpc.CallOption) (*application.ApplicationSyncWindowsResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *application.ApplicationSyncWindowsResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationSyncWindowsQuery, ...grpc.CallOption) (*application.ApplicationSyncWindowsResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationSyncWindowsQuery, ...grpc.CallOption) *application.ApplicationSyncWindowsResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*application.ApplicationSyncWindowsResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationSyncWindowsQuery, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetManifests provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) GetManifests(ctx context.Context, in *application.ApplicationManifestQuery, opts ...grpc.CallOption) (*apiclient.ManifestResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *apiclient.ManifestResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationManifestQuery, ...grpc.CallOption) (*apiclient.ManifestResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationManifestQuery, ...grpc.CallOption) *apiclient.ManifestResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*apiclient.ManifestResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationManifestQuery, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetManifestsWithFiles provides a mock function with given fields: ctx, opts
func (_m *ApplicationServiceClient) GetManifestsWithFiles(ctx context.Context, opts ...grpc.CallOption) (application.ApplicationService_GetManifestsWithFilesClient, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 application.ApplicationService_GetManifestsWithFilesClient
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, ...grpc.CallOption) (application.ApplicationService_GetManifestsWithFilesClient, error)); ok {
		return rf(ctx, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, ...grpc.CallOption) application.ApplicationService_GetManifestsWithFilesClient); ok {
		r0 = rf(ctx, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(application.ApplicationService_GetManifestsWithFilesClient)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetResource provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) GetResource(ctx context.Context, in *application.ApplicationResourceRequest, opts ...grpc.CallOption) (*application.ApplicationResourceResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *application.ApplicationResourceResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationResourceRequest, ...grpc.CallOption) (*application.ApplicationResourceResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationResourceRequest, ...grpc.CallOption) *application.ApplicationResourceResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*application.ApplicationResourceResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationResourceRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// List provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) List(ctx context.Context, in *application.ApplicationQuery, opts ...grpc.CallOption) (*v1alpha1.ApplicationList, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *v1alpha1.ApplicationList
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationQuery, ...grpc.CallOption) (*v1alpha1.ApplicationList, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationQuery, ...grpc.CallOption) *v1alpha1.ApplicationList); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1alpha1.ApplicationList)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationQuery, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListLinks provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) ListLinks(ctx context.Context, in *application.ListAppLinksRequest, opts ...grpc.CallOption) (*application.LinksResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *application.LinksResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ListAppLinksRequest, ...grpc.CallOption) (*application.LinksResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ListAppLinksRequest, ...grpc.CallOption) *application.LinksResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*application.LinksResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ListAppLinksRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListResourceActions provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) ListResourceActions(ctx context.Context, in *application.ApplicationResourceRequest, opts ...grpc.CallOption) (*application.ResourceActionsListResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *application.ResourceActionsListResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationResourceRequest, ...grpc.CallOption) (*application.ResourceActionsListResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationResourceRequest, ...grpc.CallOption) *application.ResourceActionsListResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*application.ResourceActionsListResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationResourceRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListResourceEvents provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) ListResourceEvents(ctx context.Context, in *application.ApplicationResourceEventsQuery, opts ...grpc.CallOption) (*v1.EventList, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *v1.EventList
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationResourceEventsQuery, ...grpc.CallOption) (*v1.EventList, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationResourceEventsQuery, ...grpc.CallOption) *v1.EventList); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.EventList)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationResourceEventsQuery, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListResourceLinks provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) ListResourceLinks(ctx context.Context, in *application.ApplicationResourceRequest, opts ...grpc.CallOption) (*application.LinksResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *application.LinksResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationResourceRequest, ...grpc.CallOption) (*application.LinksResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationResourceRequest, ...grpc.CallOption) *application.LinksResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*application.LinksResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationResourceRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ManagedResources provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) ManagedResources(ctx context.Context, in *application.ResourcesQuery, opts ...grpc.CallOption) (*application.ManagedResourcesResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *application.ManagedResourcesResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ResourcesQuery, ...grpc.CallOption) (*application.ManagedResourcesResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ResourcesQuery, ...grpc.CallOption) *application.ManagedResourcesResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*application.ManagedResourcesResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ResourcesQuery, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Patch provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) Patch(ctx context.Context, in *application.ApplicationPatchRequest, opts ...grpc.CallOption) (*v1alpha1.Application, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *v1alpha1.Application
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationPatchRequest, ...grpc.CallOption) (*v1alpha1.Application, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationPatchRequest, ...grpc.CallOption) *v1alpha1.Application); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1alpha1.Application)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationPatchRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// PatchResource provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) PatchResource(ctx context.Context, in *application.ApplicationResourcePatchRequest, opts ...grpc.CallOption) (*application.ApplicationResourceResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *application.ApplicationResourceResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationResourcePatchRequest, ...grpc.CallOption) (*application.ApplicationResourceResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationResourcePatchRequest, ...grpc.CallOption) *application.ApplicationResourceResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*application.ApplicationResourceResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationResourcePatchRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// PodLogs provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) PodLogs(ctx context.Context, in *application.ApplicationPodLogsQuery, opts ...grpc.CallOption) (application.ApplicationService_PodLogsClient, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 application.ApplicationService_PodLogsClient
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationPodLogsQuery, ...grpc.CallOption) (application.ApplicationService_PodLogsClient, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationPodLogsQuery, ...grpc.CallOption) application.ApplicationService_PodLogsClient); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(application.ApplicationService_PodLogsClient)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationPodLogsQuery, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ResourceTree provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) ResourceTree(ctx context.Context, in *application.ResourcesQuery, opts ...grpc.CallOption) (*v1alpha1.ApplicationTree, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *v1alpha1.ApplicationTree
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ResourcesQuery, ...grpc.CallOption) (*v1alpha1.ApplicationTree, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ResourcesQuery, ...grpc.CallOption) *v1alpha1.ApplicationTree); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1alpha1.ApplicationTree)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ResourcesQuery, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// RevisionChartDetails provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) RevisionChartDetails(ctx context.Context, in *application.RevisionMetadataQuery, opts ...grpc.CallOption) (*v1alpha1.ChartDetails, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *v1alpha1.ChartDetails
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.RevisionMetadataQuery, ...grpc.CallOption) (*v1alpha1.ChartDetails, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.RevisionMetadataQuery, ...grpc.CallOption) *v1alpha1.ChartDetails); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1alpha1.ChartDetails)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.RevisionMetadataQuery, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// RevisionMetadata provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) RevisionMetadata(ctx context.Context, in *application.RevisionMetadataQuery, opts ...grpc.CallOption) (*v1alpha1.RevisionMetadata, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *v1alpha1.RevisionMetadata
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.RevisionMetadataQuery, ...grpc.CallOption) (*v1alpha1.RevisionMetadata, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.RevisionMetadataQuery, ...grpc.CallOption) *v1alpha1.RevisionMetadata); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1alpha1.RevisionMetadata)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.RevisionMetadataQuery, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Rollback provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) Rollback(ctx context.Context, in *application.ApplicationRollbackRequest, opts ...grpc.CallOption) (*v1alpha1.Application, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *v1alpha1.Application
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationRollbackRequest, ...grpc.CallOption) (*v1alpha1.Application, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationRollbackRequest, ...grpc.CallOption) *v1alpha1.Application); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1alpha1.Application)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationRollbackRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// RunResourceAction provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) RunResourceAction(ctx context.Context, in *application.ResourceActionRunRequest, opts ...grpc.CallOption) (*application.ApplicationResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *application.ApplicationResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ResourceActionRunRequest, ...grpc.CallOption) (*application.ApplicationResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ResourceActionRunRequest, ...grpc.CallOption) *application.ApplicationResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*application.ApplicationResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ResourceActionRunRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Sync provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) Sync(ctx context.Context, in *application.ApplicationSyncRequest, opts ...grpc.CallOption) (*v1alpha1.Application, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *v1alpha1.Application
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationSyncRequest, ...grpc.CallOption) (*v1alpha1.Application, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationSyncRequest, ...grpc.CallOption) *v1alpha1.Application); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1alpha1.Application)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationSyncRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// TerminateOperation provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) TerminateOperation(ctx context.Context, in *application.OperationTerminateRequest, opts ...grpc.CallOption) (*application.OperationTerminateResponse, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *application.OperationTerminateResponse
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.OperationTerminateRequest, ...grpc.CallOption) (*application.OperationTerminateResponse, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.OperationTerminateRequest, ...grpc.CallOption) *application.OperationTerminateResponse); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*application.OperationTerminateResponse)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.OperationTerminateRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Update provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) Update(ctx context.Context, in *application.ApplicationUpdateRequest, opts ...grpc.CallOption) (*v1alpha1.Application, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *v1alpha1.Application
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationUpdateRequest, ...grpc.CallOption) (*v1alpha1.Application, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationUpdateRequest, ...grpc.CallOption) *v1alpha1.Application); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1alpha1.Application)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationUpdateRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateSpec provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) UpdateSpec(ctx context.Context, in *application.ApplicationUpdateSpecRequest, opts ...grpc.CallOption) (*v1alpha1.ApplicationSpec, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 *v1alpha1.ApplicationSpec
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationUpdateSpecRequest, ...grpc.CallOption) (*v1alpha1.ApplicationSpec, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationUpdateSpecRequest, ...grpc.CallOption) *v1alpha1.ApplicationSpec); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1alpha1.ApplicationSpec)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationUpdateSpecRequest, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Watch provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) Watch(ctx context.Context, in *application.ApplicationQuery, opts ...grpc.CallOption) (application.ApplicationService_WatchClient, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 application.ApplicationService_WatchClient
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationQuery, ...grpc.CallOption) (application.ApplicationService_WatchClient, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ApplicationQuery, ...grpc.CallOption) application.ApplicationService_WatchClient); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(application.ApplicationService_WatchClient)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ApplicationQuery, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// WatchResourceTree provides a mock function with given fields: ctx, in, opts
func (_m *ApplicationServiceClient) WatchResourceTree(ctx context.Context, in *application.ResourcesQuery, opts ...grpc.CallOption) (application.ApplicationService_WatchResourceTreeClient, error) {
	_va := make([]interface{}, len(opts))
	for _i := range opts {
		_va[_i] = opts[_i]
	}
	var _ca []interface{}
	_ca = append(_ca, ctx, in)
	_ca = append(_ca, _va...)
	ret := _m.Called(_ca...)

	var r0 application.ApplicationService_WatchResourceTreeClient
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, *application.ResourcesQuery, ...grpc.CallOption) (application.ApplicationService_WatchResourceTreeClient, error)); ok {
		return rf(ctx, in, opts...)
	}
	if rf, ok := ret.Get(0).(func(context.Context, *application.ResourcesQuery, ...grpc.CallOption) application.ApplicationService_WatchResourceTreeClient); ok {
		r0 = rf(ctx, in, opts...)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(application.ApplicationService_WatchResourceTreeClient)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, *application.ResourcesQuery, ...grpc.CallOption) error); ok {
		r1 = rf(ctx, in, opts...)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// NewApplicationServiceClient creates a new instance of ApplicationServiceClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewApplicationServiceClient(t interface {
	mock.TestingT
	Cleanup(func())
}) *ApplicationServiceClient {
	mock := &ApplicationServiceClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
