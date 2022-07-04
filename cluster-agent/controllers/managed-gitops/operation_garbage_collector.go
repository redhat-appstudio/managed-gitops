/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"time"

	"github.com/redhat-appstudio/managed-gitops/backend-shared/config/db"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Add a new row 'gc_expiration_time' for operations in the DB
// If null it will gc'ed manually.
// DB function that returns all rows with non-null `gc_expiration_time` and which are in 'Completed'/'Failed' state
// Delete the row if last_state_update + gc_expiration_time < time.Now()
// Run the above functionality in a goroutine
// Unit tests for above

const (
	garbageCollectionInterval = 10 * time.Minute
)

type garbageCollector struct {
	db db.DatabaseQueries
}

// NewGarbageCollector creates a new instance of garbageCollector
func NewGarbageCollector(dbQueries db.DatabaseQueries) *garbageCollector {
	return &garbageCollector{
		db: dbQueries,
	}
}

func (g *garbageCollector) StartGarbageCollector() {
	g.startGarbageCollectionCycle()
}

func (g *garbageCollector) startGarbageCollectionCycle() {
	func() {
		for {
			// garbage collect the operations after a specified interval
			time.After(garbageCollectionInterval)

			ctx := context.Background()
			log := log.FromContext(ctx)
			// get the  failed/completed operations with non-null gc time

			operations := []db.Operation{}
			// handle panics here
			err := g.db.ListOperationsToBeGarbageCollected(&operations)
			if err != nil {
				log.Error(err, "failed to list operations ready for garbage collected")
			}

			g.garbageCollectOperations(operations)
		}
	}()
}

func (g *garbageCollector) garbageCollectOperations(operations []db.Operation) {
	for _, operation := range operations {
		// check if the order of operations are right
		// last_state_update + gc_expiration_time < time.Now
		if operation.Last_state_update.Add(operation.GC_expiration_time).Before(time.Now()) {
			_, err := g.db.DeleteOperationById(context.Background(), operation.Operation_id)
			if err != nil {

			}
		}
	}
}

// Questions

// 1. Why can't the operation controller delete the operation row?
// 2. last_state_update + gc_expiration_time < time.Now ??
