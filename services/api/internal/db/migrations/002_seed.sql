-- Demo data (idempotent)

INSERT INTO branches (name, slug, address, timezone)
VALUES
  ('Spa Chi Nhánh Quận 1', 'spa-q1', '123 Đường ABC, Q1, TP.HCM', 'Asia/Ho_Chi_Minh'),
  ('Spa Chi Nhánh Thảo Điền', 'spa-thao-dien', '456 Đường XYZ, Thảo Điền', 'Asia/Ho_Chi_Minh')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO service_categories (name, sort_order)
SELECT v.name, v.sort_order
FROM (VALUES
  ('Chăm sóc da', 1),
  ('Massage', 2),
  ('Trị liệu', 3)
) AS v(name, sort_order)
WHERE NOT EXISTS (SELECT 1 FROM service_categories sc WHERE sc.name = v.name);

INSERT INTO services (branch_id, category_id, name, description, duration_minutes, price_cents)
SELECT b.id, c.id, 'Facial sáng da', 'Làm sạch và dưỡng ẩm', 60, 450000
FROM branches b
CROSS JOIN service_categories c
WHERE b.slug = 'spa-q1' AND c.name = 'Chăm sóc da'
  AND NOT EXISTS (
    SELECT 1 FROM services s WHERE s.branch_id = b.id AND s.name = 'Facial sáng da'
  );

INSERT INTO services (branch_id, category_id, name, description, duration_minutes, price_cents)
SELECT b.id, c.id, 'Massage thư giãn 90p', 'Toàn thân', 90, 650000
FROM branches b
CROSS JOIN service_categories c
WHERE b.slug = 'spa-q1' AND c.name = 'Massage'
  AND NOT EXISTS (
    SELECT 1 FROM services s WHERE s.branch_id = b.id AND s.name = 'Massage thư giãn 90p'
  );

INSERT INTO services (branch_id, category_id, name, description, duration_minutes, price_cents)
SELECT b.id, c.id, 'Foot reflexology', 'Chân & vai gáy', 45, 320000
FROM branches b
CROSS JOIN service_categories c
WHERE b.slug = 'spa-thao-dien' AND c.name = 'Massage'
  AND NOT EXISTS (
    SELECT 1 FROM services s WHERE s.branch_id = b.id AND s.name = 'Foot reflexology'
  );
