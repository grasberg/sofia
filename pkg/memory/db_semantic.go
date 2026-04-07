// Package memory -- semantic knowledge graph.
//
// Split across:
//   db_semantic_types.go          -- types + scanNode helper
//   db_semantic_nodes.go          -- node CRUD
//   db_semantic_edges.go          -- edge CRUD
//   db_semantic_graph.go          -- QueryGraph
//   db_semantic_stats.go          -- stale nodes, stats
//   db_semantic_consolidation.go  -- duplicates, conflicts, merge
package memory
