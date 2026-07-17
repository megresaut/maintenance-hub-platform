#!/usr/bin/env bash
# Rich mock dataset: a believable month of maintenance operations across a
# 6-property portfolio — work orders in every pipeline stage, field team
# visits, recurring series, and automated vendor-outreach threads with
# replies. Resets all business data (keeps org 1 + login).
set -euo pipefail
DB=${DB:-maintenance_hub_local}

psql -d $DB -v ON_ERROR_STOP=1 << 'SQL'
TRUNCATE ticket_activity, vendor_outreach_replies, vendor_outreach_requests, ai_drafts,
  sms_messages, sms_conversations, work_order_attachments, work_order_notes, work_orders,
  field_team_orders, tasks, task_categories, recurring_task_series, preferred_vendors,
  vendors, properties, calendar_events RESTART IDENTITY CASCADE;

-- ── Properties ────────────────────────────────────────────────────────────
INSERT INTO properties (org_id, name, address) VALUES
 (1, 'Maple Court Apartments',   '123 Main Street, Stamford, CT 06901'),
 (1, 'Oak Ridge Townhomes',      '544 Oak Ridge Road, Greenwich, CT 06830'),
 (1, 'Harborview Lofts',         '18 Harbor Street, Stamford, CT 06902'),
 (1, 'Birchwood Commons',        '77 Birchwood Avenue, Norwalk, CT 06851'),
 (1, 'The Elm District',         '210 Elm Street, Stamford, CT 06902'),
 (1, 'Ridgeline Residences',     '9 Ridgeline Drive, Greenwich, CT 06831');

-- ── Vendors ───────────────────────────────────────────────────────────────
INSERT INTO vendors (org_id, name, category, service_area_tags, phone, primary_email, notes) VALUES
 (1, 'Dolce Plumbing',                 'plumbing',    '{stamford,greenwich}', '+12035550111', 'dispatch@dolceplumbing.test',  'Fast response, preferred for emergencies'),
 (1, 'Harbor Point Plumbing & Heating','plumbing',    '{stamford,norwalk}',   '+12035550122', 'office@harborpointph.test',    NULL),
 (1, 'Coastal HVAC Services',          'hvac',        '{stamford,norwalk}',   '+12035550133', 'service@coastalhvac.test',     'Carrier certified'),
 (1, 'Apex Climate Control',           'hvac',        '{greenwich}',          '+12035550144', 'jobs@apexclimate.test',        NULL),
 (1, 'Nutmeg Electric',                'electrical',  '{greenwich,stamford}', '+12035550155', 'jobs@nutmegelectric.test',     'Licensed E-1'),
 (1, 'Brightline Electrical',          'electrical',  '{norwalk}',            '+12035550166', 'hello@brightlineelec.test',    NULL),
 (1, 'Evergreen Landscaping Co',       'landscaping', '{stamford,greenwich,norwalk}', '+12035550177', 'crew@evergreenlandscape.test', 'Seasonal contract'),
 (1, 'Summit Roofing & Gutter',        'roofing',     '{stamford,greenwich}', '+12035550188', 'estimates@summitroofing.test', NULL),
 (1, 'Sparkle Janitorial',             'cleaning',    '{stamford,norwalk}',   '+12035550199', 'book@sparklejan.test',         'Common-area turns'),
 (1, 'Shoreline Pest Solutions',       'pest_control','{stamford,greenwich,norwalk}', '+12035550200', 'help@shorelinepest.test', NULL);

-- ── Preferred vendor lists (some property+trade combos; others rely on lookup)
INSERT INTO preferred_vendors (org_id, property_id, category, vendor_id, priority) VALUES
 (1, 1, 'plumbing',   1, 1), (1, 1, 'plumbing',   2, 2),
 (1, 1, 'hvac',       3, 1),
 (1, 2, 'electrical', 5, 1),
 (1, 3, 'plumbing',   2, 1),
 (1, 5, 'cleaning',   9, 1);

-- ── Task categories ───────────────────────────────────────────────────────
INSERT INTO task_categories (org_id, name) VALUES
 (1,'Plumbing'),(1,'HVAC'),(1,'Electrical'),(1,'Roofing'),(1,'Landscaping'),(1,'Cleaning'),(1,'Pest Control'),(1,'General');

-- ── Tasks (parents for the work below) ────────────────────────────────────
INSERT INTO tasks (org_id, property_id, name, description, status, priority, category, date_requested) VALUES
 (1, 1, 'Ceiling Leak - Unit 2B',        'Water dripping through kitchen ceiling, worsening',            'in_progress', 'urgent', 'plumbing',    now() - interval '6 days'),
 (1, 2, 'Clubhouse Electrical Failure',  'Half the outlets dead, breaker trips repeatedly',              'pending_vendor', 'high','electrical', now() - interval '2 days'),
 (1, 3, 'Boiler Pressure Drop',          'Boiler losing pressure daily, units 3-9 lukewarm water',       'pending_vendor', 'high','plumbing',   now() - interval '1 day'),
 (1, 4, 'Lobby AC Blowing Warm',         'Package unit on roof short-cycling, lobby at 81F',             'in_progress', 'high',  'hvac',        now() - interval '4 days'),
 (1, 5, 'Hallway Lights Flickering',     '3rd floor corridor lights strobe intermittently',              'open',        'medium','electrical',  now() - interval '10 hours'),
 (1, 6, 'Gutter Overflow at North Wing', 'Overflowing gutters staining facade after rain',               'scheduled',   'medium','roofing',     now() - interval '8 days'),
 (1, 1, 'Dryer Vent Cleaning - Bldg A',  'Annual dryer vent cleaning, 24 units',                         'completed',   'low',   'cleaning',    now() - interval '20 days'),
 (1, 2, 'Spring Irrigation Startup',     'De-winterize irrigation, check zones',                         'completed',   'medium','landscaping', now() - interval '15 days'),
 (1, 3, 'Ant Activity in Mailroom',      'Tenant reports ants around mailroom baseboards',               'completed',   'medium','pest_control',now() - interval '12 days');

-- ── Work orders across the pipeline ──────────────────────────────────────
-- 1 dispatched (visit upcoming), 2 sourcing w/ replies, 3 sourcing awaiting,
-- 4 dispatched (visit later this week), 5 new, 6 scheduled, 7-9 completed history.
INSERT INTO work_orders (org_id, task_id, property_id, vendor_id, name, work_description, status, priority, category,
                         due_date, event_start_at, event_end_at, dispatched_at, quote_amount_cents, created_at) VALUES
 (1, 1, 1, 1, 'Ceiling Leak Repair - Unit 2B', 'Locate and repair supply-line leak above kitchen ceiling; patch drywall.', 'dispatched', 'urgent', 'plumbing',
   NULL, date_trunc('day', now()) + interval '1 day 8 hours', date_trunc('day', now()) + interval '1 day 10 hours', now() - interval '5 days', 45000, now() - interval '6 days'),
 (1, 2, 2, NULL, 'Clubhouse Outlet & Breaker Repair', 'Diagnose tripping breaker; repair dead outlet circuit in clubhouse.', 'sent', 'high', 'electrical',
   date_trunc('day', now()) + interval '4 days', NULL, NULL, NULL, NULL, now() - interval '2 days'),
 (1, 3, 3, NULL, 'Boiler Pressure Diagnosis', 'Find pressure loss source; likely expansion tank or auto-fill valve.', 'sent', 'high', 'plumbing',
   date_trunc('day', now()) + interval '3 days', NULL, NULL, NULL, NULL, now() - interval '1 day'),
 (1, 4, 4, 3, 'Lobby RTU Repair', 'Rooftop package unit short-cycling; replace contactor / check charge.', 'dispatched', 'high', 'hvac',
   NULL, date_trunc('day', now()) + interval '3 days 13 hours', date_trunc('day', now()) + interval '3 days 15 hours', now() - interval '2 days', 68000, now() - interval '4 days'),
 (1, 5, 5, NULL, 'Corridor Lighting Repair - 3rd Fl', 'Trace flicker on corridor lighting circuit; replace ballasts/drivers as needed.', 'new', 'medium', 'electrical',
   date_trunc('day', now()) + interval '6 days', NULL, NULL, NULL, NULL, now() - interval '10 hours'),
 (1, 6, 6, 8, 'Gutter Clearing & Realignment', 'Clear north-wing gutters, realign two sagging runs, check downspouts.', 'scheduled', 'medium', 'roofing',
   NULL, date_trunc('day', now()) + interval '8 days 9 hours', date_trunc('day', now()) + interval '8 days 12 hours', now() - interval '6 days', 92500, now() - interval '8 days'),
 (1, 7, 1, 9, 'Dryer Vent Cleaning - Bldg A', 'Annual dryer vent cleaning for 24 units.', 'completed', 'low', 'cleaning',
   NULL, date_trunc('day', now()) - interval '9 days' + interval '9 hours', date_trunc('day', now()) - interval '9 days' + interval '14 hours', now() - interval '18 days', 120000, now() - interval '20 days'),
 (1, 8, 2, 7, 'Irrigation Spring Startup', 'De-winterize, pressurize, inspect all 12 zones, replace 3 heads.', 'completed', 'medium', 'landscaping',
   NULL, date_trunc('day', now()) - interval '6 days' + interval '8 hours', date_trunc('day', now()) - interval '6 days' + interval '11 hours', now() - interval '13 days', 38500, now() - interval '15 days'),
 (1, 9, 3, 10, 'Mailroom Ant Treatment', 'Perimeter + baseboard gel bait treatment, follow-up in 2 weeks.', 'completed', 'medium', 'pest_control',
   NULL, date_trunc('day', now()) - interval '4 days' + interval '10 hours', date_trunc('day', now()) - interval '4 days' + interval '11 hours', now() - interval '11 days', 22500, now() - interval '12 days');
UPDATE work_orders SET completed_at = event_end_at WHERE status = 'completed';

-- ── Field team orders (on-site visits on the calendar) ────────────────────
INSERT INTO field_team_orders (org_id, task_id, property_id, name, description, status, priority,
                               event_start_at, event_end_at, field_team_member_ids, created_at) VALUES
 (1, 1, 1, 'Meet Dolce Plumbing on-site - Unit 2B', 'Let plumber into unit 2B, document repair, photos before/after', 'scheduled', 'urgent',
   date_trunc('day', now()) + interval '1 day 8 hours',  date_trunc('day', now()) + interval '1 day 10 hours', '{1}', now() - interval '5 days'),
 (1, 4, 4, 'Roof access for Coastal HVAC',          'Unlock roof hatch, supervise RTU repair',                        'scheduled', 'high',
   date_trunc('day', now()) + interval '3 days 13 hours', date_trunc('day', now()) + interval '3 days 15 hours', '{1}', now() - interval '2 days'),
 (1, 5, 5, 'Corridor lighting walkthrough',         'Video the flicker pattern for the electrician, check panel',     'new', 'medium',
   date_trunc('day', now()) + interval '2 days 9 hours',  date_trunc('day', now()) + interval '2 days 10 hours', '{1}', now() - interval '10 hours'),
 (1, 3, 3, 'Boiler room check - daily log',         'Record boiler pressure morning/evening until vendor visit',      'in_progress', 'high',
   date_trunc('day', now()) + interval '9 hours',    date_trunc('day', now()) + interval '9 hours 30 minutes', '{1}', now() - interval '1 day'),
 (1, 9, 3, 'Mailroom follow-up inspection',         'Confirm no ant activity after treatment',                        'scheduled', 'low',
   date_trunc('day', now()) + interval '10 days 10 hours', date_trunc('day', now()) + interval '10 days 10 hours 30 minutes', '{1}', now() - interval '4 days');

-- ── Recurring series ──────────────────────────────────────────────────────
INSERT INTO recurring_task_series (org_id, property_id, name, description, priority, category,
                                   frequency, interval, start_date, active, next_run_at) VALUES
 (1, 1, 'Monthly Common-Area Deep Clean',   'Lobby, halls, laundry rooms',            'medium', 'cleaning',    'monthly', 1, now() - interval '90 days', true, date_trunc('day', now()) + interval '12 days'),
 (1, 4, 'Quarterly HVAC Filter Swap',       'All RTU + common-area filters',          'medium', 'hvac',        'monthly', 3, now() - interval '200 days', true, date_trunc('day', now()) + interval '18 days'),
 (1, 6, 'Weekly Landscaping Visit',         'Mow, edge, blow walkways',               'low',    'landscaping', 'weekly',  1, now() - interval '60 days', true, date_trunc('day', now()) + interval '5 days'),
 (1, 3, 'Annual Backflow Test',             'City-required backflow certification',   'high',   'plumbing',    'yearly',  1, now() - interval '300 days', true, date_trunc('day', now()) + interval '25 days');

-- ── Outreach threads ──────────────────────────────────────────────────────
-- WO 1 (dispatched): full preferred-list thread, winner selected.
INSERT INTO vendor_outreach_requests (org_id, work_order_id, vendor_id, channel, to_address, message_body, status, sent_at, provider_ref) VALUES
 (1, 1, 1, 'sms', '+12035550111', 'Hi Dolce Plumbing, this is Demo Property Management. We need plumbing work at 123 Main Street, Stamford, CT 06901.

Job: Ceiling Leak Repair - Unit 2B — Locate and repair supply-line leak above kitchen ceiling; patch drywall.

Can you take this job? Please reply with your availability and a quote. Reference: WO-1', 'selected', now() - interval '5 days 2 hours', 'SMmock001'),
 (1, 1, 2, 'sms', '+12035550122', 'Hi Harbor Point Plumbing & Heating, this is Demo Property Management. We need plumbing work at 123 Main Street, Stamford, CT 06901.

Job: Ceiling Leak Repair - Unit 2B — Locate and repair supply-line leak above kitchen ceiling; patch drywall.

Can you take this job? Please reply with your availability and a quote. Reference: WO-1', 'replied', now() - interval '5 days 2 hours', 'SMmock002');
INSERT INTO vendor_outreach_replies (outreach_request_id, body, parsed_quote_cents, parsed_availability, raw_source_ref, received_at) VALUES
 (1, 'Hi, Joe from Dolce Plumbing. We can take the ceiling leak job — tomorrow 8am works. Quote is $450 for diagnosis and repair, parts included. Ref WO-1', 45000, 'tomorrow 8am', 'sms:+12035550111', now() - interval '5 days 1 hour'),
 (2, 'Harbor Point here. Earliest is Monday. Ballpark $600-700 depending on ceiling access. Ref WO-1', 60000, 'Monday', 'sms:+12035550122', now() - interval '5 days');

-- WO 2 (sourcing): preferred electrician replied, second local vendor quiet.
INSERT INTO vendor_outreach_requests (org_id, work_order_id, vendor_id, channel, to_address, message_body, status, sent_at, provider_ref) VALUES
 (1, 2, 5, 'sms', '+12035550155', 'Hi Nutmeg Electric, this is Demo Property Management. We need electrical work at 544 Oak Ridge Road, Greenwich, CT 06830.

Job: Clubhouse Outlet & Breaker Repair — Diagnose tripping breaker; repair dead outlet circuit in clubhouse.

Can you take this job? Please reply with your availability and a quote. Reference: WO-2', 'replied', now() - interval '1 day 6 hours', 'SMmock003'),
 (1, 2, 6, 'email', 'hello@brightlineelec.test', 'Hi Brightline Electrical, this is Demo Property Management. We need electrical work at 544 Oak Ridge Road, Greenwich, CT 06830.

Job: Clubhouse Outlet & Breaker Repair — Diagnose tripping breaker; repair dead outlet circuit in clubhouse.

Can you take this job? Please reply with your availability and a quote. Reference: WO-2', 'sent', now() - interval '1 day 6 hours', 'simulated');
INSERT INTO vendor_outreach_replies (outreach_request_id, body, parsed_quote_cents, parsed_availability, raw_source_ref, received_at) VALUES
 (3, 'Nutmeg Electric — we can be out Thursday morning. $325 diagnostic + repair, panel work extra if the breaker itself is bad. Ref WO-2', 32500, 'Thursday morning', 'sms:+12035550155', now() - interval '22 hours');

-- WO 3 (sourcing): two plumbers pinged, both replied — decision pending.
INSERT INTO vendor_outreach_requests (org_id, work_order_id, vendor_id, channel, to_address, message_body, status, sent_at, provider_ref) VALUES
 (1, 3, 2, 'sms', '+12035550122', 'Hi Harbor Point Plumbing & Heating, this is Demo Property Management. We need plumbing work at 18 Harbor Street, Stamford, CT 06902.

Job: Boiler Pressure Diagnosis — Find pressure loss source; likely expansion tank or auto-fill valve.

Can you take this job? Please reply with your availability and a quote. Reference: WO-3', 'replied', now() - interval '20 hours', 'SMmock004'),
 (1, 3, 1, 'sms', '+12035550111', 'Hi Dolce Plumbing, this is Demo Property Management. We need plumbing work at 18 Harbor Street, Stamford, CT 06902.

Job: Boiler Pressure Diagnosis — Find pressure loss source; likely expansion tank or auto-fill valve.

Can you take this job? Please reply with your availability and a quote. Reference: WO-3', 'replied', now() - interval '20 hours', 'SMmock005');
INSERT INTO vendor_outreach_replies (outreach_request_id, body, parsed_quote_cents, parsed_availability, raw_source_ref, received_at) VALUES
 (5, 'Harbor Point: boiler work is our specialty. Wednesday 7:30am, $780 if it''s the expansion tank (tank + labor). Ref WO-3', 78000, 'Wednesday 7:30am', 'sms:+12035550122', now() - interval '16 hours'),
 (6, 'Dolce Plumbing — can look Friday. Diagnosis $150, repair quoted after. Ref WO-3', 15000, 'Friday', 'sms:+12035550111', now() - interval '12 hours');

-- WO 6 (scheduled roofing): historical thread, winner picked earlier.
INSERT INTO vendor_outreach_requests (org_id, work_order_id, vendor_id, channel, to_address, message_body, status, sent_at, provider_ref) VALUES
 (1, 6, 8, 'email', 'estimates@summitroofing.test', 'Hi Summit Roofing & Gutter, this is Demo Property Management. We need roofing work at 9 Ridgeline Drive, Greenwich, CT 06831.

Job: Gutter Clearing & Realignment — Clear north-wing gutters, realign two sagging runs, check downspouts.

Can you take this job? Please reply with your availability and a quote. Reference: WO-6', 'selected', now() - interval '7 days', 'simulated');
INSERT INTO vendor_outreach_replies (outreach_request_id, body, parsed_quote_cents, parsed_availability, raw_source_ref, received_at) VALUES
 (7, 'Thanks for reaching out — we can do the north wing next week. $925 for clearing, realignment of the two runs, and downspout check. Ref WO-6', 92500, 'next week', 'email:estimates@summitroofing.test', now() - interval '6 days 20 hours');

-- ── Activity feeds ────────────────────────────────────────────────────────
INSERT INTO ticket_activity (org_id, ticket_type, ticket_id, kind, actor_type, actor_id, actor_name, body, created_at) VALUES
 (1,'work_order',1,'created','ai',NULL,'AI Intake','Work order "Ceiling Leak Repair - Unit 2B" drafted from tenant SMS and approved', now() - interval '6 days'),
 (1,'work_order',1,'outreach_sent','user',1,NULL,'Outreach sent to 2 vendor(s) (preferred_list): Dolce Plumbing via sms, Harbor Point Plumbing & Heating via sms', now() - interval '5 days 2 hours'),
 (1,'work_order',1,'outreach_reply','vendor',NULL,'Dolce Plumbing','Dolce Plumbing replied — quoted $450.00: tomorrow 8am works, parts included', now() - interval '5 days 1 hour'),
 (1,'work_order',1,'outreach_reply','vendor',NULL,'Harbor Point Plumbing & Heating','Harbor Point replied — quoted $600.00: earliest Monday', now() - interval '5 days'),
 (1,'work_order',1,'dispatched','user',1,NULL,'Dispatched to Dolce Plumbing at $450.00. Confirmed with tenant for tomorrow 8am.', now() - interval '5 days'),
 (1,'work_order',1,'note','user',1,NULL,'Tenant confirmed access; lockbox code shared with Dolce.', now() - interval '4 days'),
 (1,'work_order',2,'created','ai',NULL,'AI Intake','Work order "Clubhouse Outlet & Breaker Repair" drafted from tenant SMS and approved', now() - interval '2 days'),
 (1,'work_order',2,'outreach_sent','user',1,NULL,'Outreach sent to 2 vendor(s) (preferred_list): Nutmeg Electric via sms, Brightline Electrical via email', now() - interval '1 day 6 hours'),
 (1,'work_order',2,'outreach_reply','vendor',NULL,'Nutmeg Electric','Nutmeg Electric replied — quoted $325.00: Thursday morning', now() - interval '22 hours'),
 (1,'work_order',3,'created','user',1,NULL,'Work order "Boiler Pressure Diagnosis" created', now() - interval '1 day'),
 (1,'work_order',3,'outreach_sent','user',1,NULL,'Outreach sent to 2 vendor(s) (preferred_list): Harbor Point Plumbing & Heating via sms, Dolce Plumbing via sms', now() - interval '20 hours'),
 (1,'work_order',3,'outreach_reply','vendor',NULL,'Harbor Point Plumbing & Heating','Harbor Point replied — quoted $780.00: Wednesday 7:30am', now() - interval '16 hours'),
 (1,'work_order',3,'outreach_reply','vendor',NULL,'Dolce Plumbing','Dolce Plumbing replied — quoted $150.00 diagnosis: Friday', now() - interval '12 hours'),
 (1,'work_order',4,'created','ai',NULL,'AI Intake','Work order "Lobby RTU Repair" drafted from Outlook event and approved', now() - interval '4 days'),
 (1,'work_order',4,'dispatched','user',1,NULL,'Dispatched to Coastal HVAC Services at $680.00.', now() - interval '2 days'),
 (1,'work_order',5,'created','ai',NULL,'AI Intake','Work order "Corridor Lighting Repair - 3rd Fl" drafted from tenant SMS and approved', now() - interval '10 hours'),
 (1,'work_order',6,'dispatched','user',1,NULL,'Dispatched to Summit Roofing & Gutter at $925.00.', now() - interval '6 days'),
 (1,'work_order',7,'status_change','user',1,NULL,'Status changed from dispatched to completed', now() - interval '9 days'),
 (1,'work_order',8,'status_change','user',1,NULL,'Status changed from dispatched to completed', now() - interval '6 days'),
 (1,'work_order',9,'status_change','user',1,NULL,'Status changed from dispatched to completed', now() - interval '4 days'),
 (1,'fto',1,'created','system',NULL,NULL,'Field visit scheduled to meet Dolce Plumbing on-site', now() - interval '5 days'),
 (1,'fto',4,'status_change','user',1,NULL,'Status changed from new to in_progress', now() - interval '20 hours');

-- ── An SMS conversation so the intake viewer has content ─────────────────
INSERT INTO sms_conversations (org_id, phone_number, last_message_at, last_classified_at) VALUES
 (1, '+12035559999', now() - interval '10 hours', now() - interval '10 hours');
INSERT INTO sms_messages (conversation_id, direction, from_number, to_number, body, processed, created_at) VALUES
 (1,'inbound','+12035559999','+18559255335','Hi, tenant in unit 2B at 123 Main Street here. Water is dripping through the kitchen ceiling and getting worse - can you send a plumber asap?', true, now() - interval '6 days'),
 (1,'outbound','+18559255335','+12035559999','Maintenance detected: Ceiling Leak - Unit 2B. Draft work order created for review.', true, now() - interval '6 days'),
 (1,'inbound','+12035559999','+18559255335','Also the hallway lights on 3 have been flickering all morning at Elm District', true, now() - interval '10 hours'),
 (1,'outbound','+18559255335','+12035559999','Maintenance detected: Hallway Lights Flickering. Draft created — vendor selection needed.', true, now() - interval '10 hours');
SQL

echo "rich demo data seeded"
