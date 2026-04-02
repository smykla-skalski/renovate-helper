package github

const prFragment = `
fragment prFields on SearchResultItemConnection {
  nodes {
    ... on PullRequest {
      id
      number
      title
      url
      state
      mergeable
      repository { nameWithOwner }
      reviewDecision
      commits(last: 1) {
        nodes {
          commit {
            statusCheckRollup {
              contexts(first: 50) {
                nodes {
                  ... on CheckRun {
                    name
                    status
                    conclusion
                    checkSuite { id }
                  }
                  ... on StatusContext {
                    context
                    state
                  }
                }
              }
            }
          }
        }
      }
      reviews(last: 10) {
        nodes {
          author { login }
          state
        }
      }
      labels(first: 10) { nodes { name } }
      additions
      deletions
      createdAt
      updatedAt
    }
  }
}
`

const mergeMutation = `
mutation MergePR($id: ID!, $method: PullRequestMergeMethod!) {
  mergePullRequest(input: { pullRequestId: $id, mergeMethod: $method }) {
    pullRequest { number state }
  }
}
`

const approveMutation = `
mutation ApprovePR($id: ID!) {
  addPullRequestReview(input: { pullRequestId: $id, event: APPROVE }) {
    pullRequestReview { state }
  }
}
`

const rerequestCheckSuiteMutation = `
mutation RerequestCheckSuite($checkSuiteId: ID!, $repositoryId: ID!) {
  rerequestCheckSuite(input: { checkSuiteId: $checkSuiteId, repositoryId: $repositoryId }) {
    checkSuite { id }
  }
}
`

const repoIDQuery = `
query RepoID($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) { id }
}
`
