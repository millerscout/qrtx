insert into public.permissions (code, name) values
  ('records.view.any', 'View all records'),
  ('records.view.own', 'View own records'),
  ('custom_field.manage.company', 'Company fields'),
  ('custom_field.manage.contact', 'Contact fields'),
  ('custom_field.manage.deal', 'Deal fields'),
  ('custom_field.manage.task', 'Task fields'),
  ('pipeline.create', 'Create pipelines'),
  ('pipeline.edit', 'Edit pipelines'),
  ('company.add', 'Add companies'),
  ('task.skip', 'Skip tasks'),
  ('settings.manage', 'Manage settings')
on conflict (code) do nothing;
