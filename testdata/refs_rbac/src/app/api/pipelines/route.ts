import { supabase } from '@/lib/supabase'

export async function GET() {
  const { data: allowed } = await supabase.rpc('user_has_permission', { code: 'pipeline.edit' })
  return allowed ? Response.json([]) : new Response(null, { status: 403 })
}
