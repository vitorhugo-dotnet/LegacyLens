SELECT o.`id`, c.name
FROM `orders` o
JOIN customers c ON c.id = o.customer_id;

UPDATE `orders` SET status = ? WHERE id = ?;
