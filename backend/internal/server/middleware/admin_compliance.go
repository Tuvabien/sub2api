package middleware

import (
	// "net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminComplianceGuard 管理员合规确认中间件
// NOTE: [2026-07-04] 已临时禁用合规确认检查，允许管理员直接访问系统
// 修改原因：开发环境下跳过合规确认流程，提升开发效率
// 修改人：solo
// 恢复方式：取消注释下方代码块，删除 c.Next() 直接返回的逻辑
func AdminComplianceGuard(settingService *service.SettingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ===== [2026-07-04] 临时禁用：直接放行所有请求 =====
		c.Next()
		return
		// ===== 以下为原有合规确认逻辑，已注释禁用 =====
		/*
			if settingService == nil || isAdminComplianceBypassPath(c.Request.URL.Path) {
				c.Next()
				return
			}

			subject, ok := GetAuthSubjectFromContext(c)
			if !ok {
				AbortWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization required")
				return
			}

			acknowledged, err := settingService.IsAdminComplianceAcknowledged(c.Request.Context(), subject.UserID)
			if err != nil {
				AbortWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
				return
			}
			if acknowledged {
				c.Next()
				return
			}

			c.JSON(http.StatusLocked, gin.H{
				"code":    "ADMIN_COMPLIANCE_ACK_REQUIRED",
				"message": "administrator compliance acknowledgement is required",
				"metadata": gin.H{
					"version":          service.AdminComplianceVersion,
					"document_path_zh": service.AdminComplianceDocumentPathZH,
					"document_path_en": service.AdminComplianceDocumentPathEN,
					"document_url_zh":  service.AdminComplianceDocumentURLZH,
					"document_url_en":  service.AdminComplianceDocumentURLEN,
				},
			})
			c.Abort()
		*/
	}
}

func isAdminComplianceBypassPath(path string) bool {
	path = strings.TrimSpace(path)
	return path == "/api/v1/admin/compliance" || strings.HasPrefix(path, "/api/v1/admin/compliance/")
}