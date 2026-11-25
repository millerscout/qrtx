import { getUserPermissions } from '@/lib/rbac'

export async function checkPermission(userId: string, permissionCode: string) {
  const { data, error } = await getUserPermissions(userId)
  if (error || !data) return { ok: false, status: 500 }
  if (!data.permissionCodes.has(permissionCode)) return { ok: false, status: 403 }
  return { ok: true, status: 200 }
}

export function withAuthAndPermission(permissionCode: string, handler: Function) {
  return async (request: Request) => {
    const res = await checkPermission('u', permissionCode)
    return res.ok ? handler(request) : res
  }
}
