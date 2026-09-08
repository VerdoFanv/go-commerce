-- Remove demo catalog rows (keep admin from 000002).

DELETE FROM products
WHERE name IN (
    'Kopi Susu Gula Aren',
    'Matcha Latte',
    'Croissant Butter',
    'Americano',
    'Brown Sugar Boba',
    'Admin Merch Hoodie',
    'Admin Sticker Pack'
);

DELETE FROM users
WHERE email IN ('seller@golang-be.dev', 'buyer@golang-be.dev');
