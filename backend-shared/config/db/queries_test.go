package db

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierr "k8s.io/apimachinery/pkg/api/errors"
)

var _ = Describe("Database Query interface tests", func() {

	Context("Test Shared Connection Pool Logic", func() {

		const sharedConnectionPoolUserName = "test-shared-pool-user-name"

		// cleanupDatabase deletes the test user created to verify the connection
		cleanupDatabase := func() {
			var err error

			// Ensure the created ClusterUser is deleted
			conn, err := NewSharedProductionPostgresDBQueries(false)
			if err != nil {
				GinkgoWriter.Println(err)
				return
			}

			createdClusterUser := ClusterUser{
				User_name: sharedConnectionPoolUserName,
			}

			err = conn.GetClusterUserByUsername(context.Background(), &createdClusterUser)
			if err != nil && !apierr.IsNotFound(err) {
				GinkgoWriter.Println(err)
				return
			}

			_, err = conn.DeleteClusterUserById(context.Background(), createdClusterUser.Clusteruser_id)
			if err != nil && !apierr.IsNotFound(err) {
				GinkgoWriter.Println(err)
				return
			}
		}

		BeforeEach(func() {
			cleanupDatabase()
		})

		AfterEach(func() {
			cleanupDatabase()
		})

		It("Should maintain only two shared connection pools: verbose and non-verbose", func() {

			By("verifying that verbose and non-verbose parameters return different pools")
			firstNonVerbose, err := NewSharedProductionPostgresDBQueries(false)
			Expect(err).To(BeNil())
			firstVerbose, err := NewSharedProductionPostgresDBQueries(true)
			Expect(err).To(BeNil())
			Expect(firstNonVerbose).ToNot(Equal(firstVerbose))

			By("verifying that each subsequent call returns the existing connection object, which confirms the connections are shared in the pool.")
			for count := 0; count < 10; count++ {
				nextVerbose, err := NewSharedProductionPostgresDBQueries(true)
				Expect(err).To(BeNil())
				nextNonVerbose, err := NewSharedProductionPostgresDBQueries(false)
				Expect(err).To(BeNil())

				Expect(nextVerbose).To(Equal(firstVerbose))
				Expect(nextNonVerbose).To(Equal(firstNonVerbose))
			}
		})

		verifyValidConnection := func(conn DatabaseQueries) {
			By("creating and geting a cluster user to verify the database connection works")
			var err error
			createdClusterUser := ClusterUser{
				User_name: sharedConnectionPoolUserName,
			}
			err = conn.CreateClusterUser(context.Background(), &createdClusterUser)
			Expect(err).To(BeNil())

			retrievedClusterUser := ClusterUser{
				Clusteruser_id: createdClusterUser.Clusteruser_id,
			}
			err = conn.GetClusterUserById(context.Background(), &retrievedClusterUser)
			Expect(err).To(BeNil())
		}

		It("Should verify the non-verbose shared connection pools can be connected to", func() {

			conn, err := NewSharedProductionPostgresDBQueries(false)
			Expect(err).To(BeNil())

			verifyValidConnection(conn)
		})

		It("Should verify the verbose shared connection pools can be connected to", func() {

			conn, err := NewSharedProductionPostgresDBQueries(true)
			Expect(err).To(BeNil())

			verifyValidConnection(conn)
		})

	})

})
