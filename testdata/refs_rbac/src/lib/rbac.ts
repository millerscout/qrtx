import { supabase } from './supabase'

export async function getUserPermissions(userId: string) {
  return { data: { permissionCodes: new Set<string>() }, error: null }
}

export async function hasPermission(userId: string, code: string) {
  const { data } = await supabase.rpc('user_has_permission', { user_id: userId, code })
  return !!data
}
