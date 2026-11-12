package com.windshift.dcimport.screen;

import com.atlassian.jira.issue.fields.OrderableField;
import com.atlassian.jira.issue.fields.screen.FieldScreen;
import com.atlassian.jira.issue.fields.screen.FieldScreenLayoutItem;
import com.atlassian.jira.issue.fields.screen.FieldScreenManager;
import com.atlassian.jira.issue.fields.screen.FieldScreenScheme;
import com.atlassian.jira.issue.fields.screen.FieldScreenSchemeItem;
import com.atlassian.jira.issue.fields.screen.FieldScreenSchemeManager;
import com.atlassian.jira.issue.fields.screen.FieldScreenTab;
import com.atlassian.jira.issue.fields.screen.issuetype.IssueTypeScreenScheme;
import com.atlassian.jira.issue.fields.screen.issuetype.IssueTypeScreenSchemeEntity;
import com.atlassian.jira.issue.fields.screen.issuetype.IssueTypeScreenSchemeManager;
import com.atlassian.jira.issue.operation.ScreenableIssueOperation;
import com.atlassian.jira.project.Project;
import com.atlassian.jira.project.ProjectManager;
import com.atlassian.plugin.spring.scanner.annotation.imports.ComponentImport;
import jakarta.inject.Inject;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.Set;

/**
 * Reads a project's effective create/edit/view screens and their ordered fields
 * from Jira's screen managers. Data Center's public REST API does not expose
 * screen schemes, so the importer cannot build faithful create/edit/view screens
 * without this.
 */
@Component
public class ScreenExtractor {

    private final ProjectManager projectManager;
    private final IssueTypeScreenSchemeManager issueTypeScreenSchemeManager;
    private final FieldScreenSchemeManager fieldScreenSchemeManager;
    private final FieldScreenManager fieldScreenManager;

    @Inject
    public ScreenExtractor(@ComponentImport ProjectManager projectManager,
                           @ComponentImport IssueTypeScreenSchemeManager issueTypeScreenSchemeManager,
                           @ComponentImport FieldScreenSchemeManager fieldScreenSchemeManager,
                           @ComponentImport FieldScreenManager fieldScreenManager) {
        this.projectManager = projectManager;
        this.issueTypeScreenSchemeManager = issueTypeScreenSchemeManager;
        this.fieldScreenSchemeManager = fieldScreenSchemeManager;
        this.fieldScreenManager = fieldScreenManager;
    }

    public Optional<Map<String, Object>> projectScreens(String projectKey) {
        Project project = projectManager.getProjectObjByKey(projectKey);
        if (project == null) {
            return Optional.empty();
        }
        IssueTypeScreenScheme scheme = issueTypeScreenSchemeManager.getIssueTypeScreenScheme(project);
        if (scheme == null) {
            return Optional.empty();
        }

        Map<String, String> screenSchemeByIssueType = new LinkedHashMap<>();
        String defaultScreenSchemeId = null;
        for (Object raw : issueTypeScreenSchemeManager.getIssueTypeScreenSchemeEntities(scheme)) {
            if (!(raw instanceof IssueTypeScreenSchemeEntity)) {
                continue;
            }
            IssueTypeScreenSchemeEntity entity = (IssueTypeScreenSchemeEntity) raw;
            FieldScreenScheme fieldScreenScheme = entity.getFieldScreenScheme();
            if (fieldScreenScheme == null || fieldScreenScheme.getId() == null) {
                continue;
            }
            String issueTypeId = entity.getIssueTypeId();
            if (issueTypeId == null || issueTypeId.isEmpty()) {
                defaultScreenSchemeId = String.valueOf(fieldScreenScheme.getId());
            } else {
                screenSchemeByIssueType.put(issueTypeId, String.valueOf(fieldScreenScheme.getId()));
            }
        }

        Set<String> screenSchemeIds = new LinkedHashSet<>(screenSchemeByIssueType.values());
        if (defaultScreenSchemeId != null) {
            screenSchemeIds.add(defaultScreenSchemeId);
        }

        Set<String> screenIds = new LinkedHashSet<>();
        Map<String, Map<String, String>> operationsBySchemeId = new LinkedHashMap<>();
        for (String screenSchemeId : screenSchemeIds) {
            FieldScreenScheme fieldScreenScheme = fieldScreenSchemeManager.getFieldScreenScheme(Long.valueOf(screenSchemeId));
            if (fieldScreenScheme == null) {
                continue;
            }
            Map<String, String> operations = new LinkedHashMap<>();
            String defaultScreenId = null;
            for (Object itemRaw : fieldScreenScheme.getFieldScreenSchemeItems()) {
                if (!(itemRaw instanceof FieldScreenSchemeItem)) {
                    continue;
                }
                FieldScreenSchemeItem item = (FieldScreenSchemeItem) itemRaw;
                FieldScreen screen = item.getFieldScreen();
                if (screen == null || screen.getId() == null) {
                    continue;
                }
                String screenId = String.valueOf(screen.getId());
                String operationKey = operationKey(item.getIssueOperation());
                if (operationKey != null) {
                    operations.put(operationKey, screenId);
                    screenIds.add(screenId);
                } else if (defaultScreenId == null) {
                    // No operation: Jira's default screen for every operation that
                    // is not mapped explicitly.
                    defaultScreenId = screenId;
                    screenIds.add(screenId);
                }
            }
            if (defaultScreenId != null) {
                for (String operation : new String[]{"createScreenId", "editScreenId", "viewScreenId"}) {
                    operations.putIfAbsent(operation, defaultScreenId);
                }
            }
            operationsBySchemeId.put(screenSchemeId, operations);
        }

        Map<String, Map<String, String>> issueTypeScreens = new LinkedHashMap<>();
        for (Map.Entry<String, String> entry : screenSchemeByIssueType.entrySet()) {
            Map<String, String> operations = operationsBySchemeId.get(entry.getValue());
            if (operations != null) {
                issueTypeScreens.put(entry.getKey(), operations);
            }
        }
        if (defaultScreenSchemeId != null) {
            Map<String, String> operations = operationsBySchemeId.get(defaultScreenSchemeId);
            if (operations != null) {
                issueTypeScreens.put("default", operations);
            }
        }

        List<Map<String, Object>> screens = new ArrayList<>();
        for (String screenId : screenIds) {
            FieldScreen screen = fieldScreenManager.getFieldScreen(Long.valueOf(screenId));
            if (screen != null) {
                screens.add(screenSummary(screen));
            }
        }

        Map<String, Object> result = new LinkedHashMap<>();
        result.put("issueTypeScreens", issueTypeScreens);
        result.put("screens", screens);
        return Optional.of(result);
    }

    private Map<String, Object> screenSummary(FieldScreen screen) {
        List<FieldScreenTab> tabs = screen.getTabs();
        List<Map<String, Object>> fields = new ArrayList<>();
        for (FieldScreenTab tab : tabs) {
            for (Object itemRaw : tab.getFieldScreenLayoutItems()) {
                if (!(itemRaw instanceof FieldScreenLayoutItem)) {
                    continue;
                }
                FieldScreenLayoutItem item = (FieldScreenLayoutItem) itemRaw;
                OrderableField orderableField = item.getOrderableField();
                Map<String, Object> field = new LinkedHashMap<>();
                field.put("id", item.getFieldId());
                field.put("name", orderableField == null ? item.getFieldId() : orderableField.getName());
                fields.add(field);
            }
        }

        Map<String, Object> summary = new LinkedHashMap<>();
        summary.put("id", String.valueOf(screen.getId()));
        summary.put("name", screen.getName());
        summary.put("description", screen.getDescription());
        summary.put("tabCount", tabs.size());
        summary.put("fields", fields);
        return summary;
    }

    private String operationKey(ScreenableIssueOperation operation) {
        if (operation == null || operation.getId() == null) {
            return null;
        }
        switch (operation.getId().intValue()) {
            case 0:
                return "createScreenId";
            case 1:
                return "editScreenId";
            case 2:
                return "viewScreenId";
            default:
                return null;
        }
    }
}
