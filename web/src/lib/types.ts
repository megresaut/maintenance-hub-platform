// Shared domain types matching the Maintenance Hub API contract.

export type Property = {
  id: number
  name: string
  address: string
  created_at: string
}

export type Vendor = {
  id: number
  name: string
  category: string
  service_area_tags: string[]
  primary_email?: string | null
  phone?: string | null
  alt_email?: string | null
  alt_phone?: string | null
  notes?: string | null
  website?: string | null
  address?: string | null
}

export type PreferredVendor = {
  id: number
  property_id: number
  category: string
  vendor_id: number
  priority: number
  vendor_name: string
  property_name: string
}

export type AIDraft = {
  id: number
  source: 'sms' | 'calendar' | 'manual'
  source_ref?: string | null
  entity_type: 'task' | 'work_order' | 'fto'
  extracted_data: Record<string, unknown>
  ai_confidence?: number | null
  ai_reasoning?: string | null
  status: string
  parent_draft_id?: number | null
  created_entity_id?: number | null
  created_at: string
  child_drafts?: AIDraft[] | null
}

export type Task = {
  id: number
  property_id: number
  property_name?: string | null
  name: string
  description: string
  category: string
  priority: string
  status: string
  created_at: string
}

export type WorkOrder = {
  id: number
  task_id?: number | null
  property_id: number
  property_name?: string | null
  vendor_id?: number | null
  name: string
  work_description: string
  status: string
  priority: string
  quote_amount_cents?: number | null
  dispatched_at?: string | null
  category?: string | null
  media_urls?: string[] | null
  due_date?: string | null
  created_at: string
}

export type FTO = {
  id: number
  name: string
  description?: string | null
  property_id?: number | null
  property_name?: string | null
  task_id?: number | null
  field_team_member_ids: number[]
  priority: string
  status: string
  dispatched_at?: string | null
  created_at: string
}

export type Activity = {
  id: number
  kind: string
  actor_type: string
  actor_name?: string | null
  body: string
  metadata?: Record<string, unknown> | null
  created_at: string
}

export type OutreachReply = {
  id: number
  body: string
  parsed_quote_cents?: number | null
  parsed_availability?: string | null
  received_at: string
}

export type OutreachRequest = {
  id: number
  vendor_id: number
  vendor_name: string
  vendor_phone?: string | null
  vendor_email?: string | null
  channel: 'sms' | 'email'
  to_address: string
  message_body: string
  status: 'pending' | 'sent' | 'failed' | 'replied' | 'selected'
  error?: string | null
  sent_at?: string | null
  replies: OutreachReply[]
}

export type Shortlist = {
  source: 'preferred_list' | 'local_lookup'
  vendors: Vendor[]
}

// SMS admin shapes are not fully specified; keep fields optional and render defensively.
export type SmsConversation = {
  id: number
  phone_number?: string | null
  from_number?: string | null
  contact_name?: string | null
  last_message?: string | null
  last_message_at?: string | null
  updated_at?: string | null
  created_at?: string | null
}

export type SmsMessage = {
  id: number
  direction?: string | null
  from_number?: string | null
  to_number?: string | null
  body?: string | null
  created_at?: string | null
}

export const CATEGORIES = [
  'plumbing',
  'electrical',
  'hvac',
  'roofing',
  'painting',
  'landscaping',
  'cleaning',
  'pest_control',
  'appliance',
  'general',
] as const

export const PRIORITIES = ['low', 'medium', 'high', 'urgent'] as const

export const WO_STATUSES = [
  'new',
  'sent',
  'dispatched',
  'scheduled',
  'in_progress',
  'completed',
  'cancelled',
  'deferred',
  'closed',
] as const

export const TASK_STATUSES = [
  'open',
  'in_progress',
  'pending_vendor',
  'scheduled',
  'completed',
  'closed',
  'cancelled',
  'deferred',
] as const

export const FTO_STATUSES = [
  'new',
  'scheduled',
  'dispatched',
  'in_progress',
  'completed',
  'cancelled',
] as const
