import { withAuthAndPermission } from '@/lib/api/utils'

export const POST = withAuthAndPermission('user.create', async () => Response.json({}))
