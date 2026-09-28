-- +goose Up
UPDATE milestone_definitions SET title='Orientation & research concept' WHERE id='71000000-0000-0000-0000-000000000001' AND title='Orientation & topic selection';
UPDATE milestone_definitions SET description='Develop your research concept and confirm supervision arrangements.' WHERE id='71200000-0000-0000-0000-000000000001' AND description='Confirm topic and supervision arrangements.';
-- +goose Down
UPDATE milestone_definitions SET title='Orientation & topic selection' WHERE id='71000000-0000-0000-0000-000000000001' AND title='Orientation & research concept';
