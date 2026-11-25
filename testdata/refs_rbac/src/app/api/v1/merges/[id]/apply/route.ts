import { supabase } from '@/lib/supabase'

export async function POST() {
  const { data: allowed } = await supabase.rpc('user_has_permission', { code: 'company.add' })
  return allowed ? Response.json({ ok: true }) : new Response(null, { status: 403 })
}
