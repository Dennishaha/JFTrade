package indicatorruntime

func buildSnapshotKeyCache(requirements indicatorRequirements) snapshotKeyCache {
	builder := newSnapshotKeyCacheBuilder(requirements)
	builder.populate()
	return builder.cache
}
