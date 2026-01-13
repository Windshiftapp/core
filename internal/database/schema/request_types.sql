
				CREATE TABLE IF NOT EXISTS request_types (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					channel_id INTEGER NOT NULL,
					name TEXT NOT NULL,
					description TEXT DEFAULT '',
					item_type_id INTEGER NOT NULL,
					icon TEXT DEFAULT 'FileText',
					color TEXT DEFAULT '#6b7280',
					display_order INTEGER DEFAULT 0,
					is_active BOOLEAN DEFAULT 1,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
					FOREIGN KEY (item_type_id) REFERENCES item_types(id) ON DELETE RESTRICT
				);
			
