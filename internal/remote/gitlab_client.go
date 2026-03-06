package remote

import (
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// GitLabRepositoryFilesService defines the GitLab Repository Files API methods we use.
type GitLabRepositoryFilesService interface {
	GetRawFile(pid interface{}, fileName string, opt *gitlab.GetRawFileOptions, options ...gitlab.RequestOptionFunc) ([]byte, *gitlab.Response, error)
	GetFile(pid interface{}, fileName string, opt *gitlab.GetFileOptions, options ...gitlab.RequestOptionFunc) (*gitlab.File, *gitlab.Response, error)
	CreateFile(pid interface{}, fileName string, opt *gitlab.CreateFileOptions, options ...gitlab.RequestOptionFunc) (*gitlab.FileInfo, *gitlab.Response, error)
	UpdateFile(pid interface{}, fileName string, opt *gitlab.UpdateFileOptions, options ...gitlab.RequestOptionFunc) (*gitlab.FileInfo, *gitlab.Response, error)
}

// GitLabRepositoriesService defines the GitLab Repositories API methods we use.
type GitLabRepositoriesService interface {
	ListTree(pid interface{}, opt *gitlab.ListTreeOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.TreeNode, *gitlab.Response, error)
}

// GitLabCommitsService defines the GitLab Commits API methods we use.
type GitLabCommitsService interface {
	GetCommit(pid interface{}, sha string, opt *gitlab.GetCommitOptions, options ...gitlab.RequestOptionFunc) (*gitlab.Commit, *gitlab.Response, error)
}

// GitLabBranchesService defines the GitLab Branches API methods we use.
type GitLabBranchesService interface {
	GetBranch(pid interface{}, branch string, options ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error)
	CreateBranch(pid interface{}, opt *gitlab.CreateBranchOptions, options ...gitlab.RequestOptionFunc) (*gitlab.Branch, *gitlab.Response, error)
}

// GitLabTagsService defines the GitLab Tags API methods we use.
type GitLabTagsService interface {
	GetTag(pid interface{}, tag string, options ...gitlab.RequestOptionFunc) (*gitlab.Tag, *gitlab.Response, error)
}

// GitLabProjectsService defines the GitLab Projects API methods we use.
type GitLabProjectsService interface {
	ListProjects(opt *gitlab.ListProjectsOptions, options ...gitlab.RequestOptionFunc) ([]*gitlab.Project, *gitlab.Response, error)
	GetProject(pid interface{}, opt *gitlab.GetProjectOptions, options ...gitlab.RequestOptionFunc) (*gitlab.Project, *gitlab.Response, error)
}

// GitLabMergeRequestsService defines the GitLab Merge Requests API methods we use.
type GitLabMergeRequestsService interface {
	CreateMergeRequest(pid interface{}, opt *gitlab.CreateMergeRequestOptions, options ...gitlab.RequestOptionFunc) (*gitlab.MergeRequest, *gitlab.Response, error)
}

// GitLabClient wraps the services we need from the GitLab client.
type GitLabClient interface {
	RepositoryFiles() GitLabRepositoryFilesService
	Repositories() GitLabRepositoriesService
	Commits() GitLabCommitsService
	Branches() GitLabBranchesService
	Tags() GitLabTagsService
	Projects() GitLabProjectsService
	MergeRequests() GitLabMergeRequestsService
}

// realGitLabClient wraps the actual gitlab.Client.
type realGitLabClient struct {
	client *gitlab.Client
}

func newRealGitLabClient(token string, opts ...gitlab.ClientOptionFunc) (GitLabClient, error) {
	client, err := gitlab.NewClient(token, opts...)
	if err != nil {
		return nil, err
	}
	return &realGitLabClient{client: client}, nil
}

func (c *realGitLabClient) RepositoryFiles() GitLabRepositoryFilesService {
	return c.client.RepositoryFiles
}

func (c *realGitLabClient) Repositories() GitLabRepositoriesService {
	return c.client.Repositories
}

func (c *realGitLabClient) Commits() GitLabCommitsService {
	return c.client.Commits
}

func (c *realGitLabClient) Branches() GitLabBranchesService {
	return c.client.Branches
}

func (c *realGitLabClient) Tags() GitLabTagsService {
	return c.client.Tags
}

func (c *realGitLabClient) Projects() GitLabProjectsService {
	return c.client.Projects
}

func (c *realGitLabClient) MergeRequests() GitLabMergeRequestsService {
	return c.client.MergeRequests
}
