package bucket

func updateAction(req updateRequest, before Bucket) string {
	if loggingChanged(req, before) {
		return "modify_access_logging"
	}
	if quotaChanged(req, before) {
		return "modify_bucket_quota"
	}
	return "modify_bucket"
}

func loggingChanged(req updateRequest, before Bucket) bool {
	if req.AccessLoggingEnabled != nil && *req.AccessLoggingEnabled != before.AccessLoggingEnabled {
		return true
	}
	if req.AccessLogTargetBucket != nil && strPtr(emptyToNil(req.AccessLogTargetBucket)) != strPtr(before.AccessLogTargetBucket) {
		return true
	}
	if req.AccessLogPrefix != nil && strPtr(emptyToNil(req.AccessLogPrefix)) != strPtr(before.AccessLogPrefix) {
		return true
	}
	return false
}

func quotaChanged(req updateRequest, before Bucket) bool {
	if req.QuotaBytes != nil && !int64PtrEq(req.QuotaBytes, before.QuotaBytes) {
		return true
	}
	if req.ObjectLimit != nil && !int64PtrEq(req.ObjectLimit, before.ObjectLimit) {
		return true
	}
	return false
}

func int64PtrEq(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
