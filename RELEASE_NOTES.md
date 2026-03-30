# Windshift v0.4.1

---

> **Not recommended for production use.**
>
> Windshift is an early release that is still undergoing internal testing. APIs, data formats, and configuration may change between releases without migration paths. We publish this release to invite early exploration, testing, and feedback - not to support production workloads.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## Highlights

### More Standardized Audit Logging
Unified audit logging mechanism for all enum-like services (e.g., Status Categories, Milestone Categories, etc.).
- **Consistent Logging:** All CRUD operations on these entities are now consistently logged with user, IP, and timestamp information.
- **Reduced Complexity:** This refactoring significantly simplifies the backend codebase and ensures easier maintenance of audit trails.

### Frontend and UI Enhancements
- **Refactored Custom Fields:** Significant updates to the `CustomFieldRenderer` for better performance and extensibility.
- **UI Polishing:** Improvements to core components like `Button`, `Tabs`, `Textarea`, and `DropdownMenu` for a more consistent user experience.
- **Internationalization:** Updated translations for Arabic, German, English, Spanish, and Brazilian Portuguese.

---

## Bug Fixes

- **Board View:** Fixed card height inconsistency where cards without an assignee avatar appeared shorter than cards with one.
- **Frontend Permissions** Made permissions consistent between backend implementation and frontend, preventing errors from bubbling up
