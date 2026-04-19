# Windshift v0.5.2

---

> **Suitable for small-scale production use.**
>
> Windshift is maturing and can now be used for small-scale production workloads. Be aware that APIs, data formats, and configuration may still change between releases without guaranteed migration paths. We recommend keeping backups and testing upgrades in a staging environment before applying them.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## Enhancements

### Frontend

- Removed duplicate code, pulled more code into standard components
- Hub Inbox did not display Requests correctly
- Replace all javascript navigation function with native links or fallbacks to native links so that Ctrl / Click works across the app
- Make dates more timezone safe

### Backend

- Hardened Item Linking permission so that Asset and Test Case permissions are also respected
- Added missing audit events (portal customer related)
- Persists is_enabled flag for assets correctly
- Added cycle detection to item hierarchy (would lead to endless loops when connecting items incorrectly via api)
- Hardened portal registration (added options for domain whitelist and no-signup option)
