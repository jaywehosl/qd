package ru.quicdiver.client;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.text.Editable;
import android.text.TextWatcher;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.View;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

public class RoutingPage {

    private static final String[] ROLES = {"direct", "noEgress", "egress", "tunnel"};
    private static final String[] NAMES = {"напрямую", "−egress", "+egress", "туннель"};

    private static final String[] BASE_ROLES = {"direct", "tunnel"};
    private static final String[] BASE_NAMES = {"напрямую", "туннель"};

    private final Activity host;
    private final Skin skin;
    private final Map<String, String> labels = new HashMap<>();

    private LinearLayout defaultRow;
    private LinearLayout list;

    private String defaultRole = "tunnel";
    private final List<Rule> rules = new ArrayList<>();
    private List<App> catalogue;
    private boolean loaded;
    private volatile boolean loading;
    private String shownRole = "";

    private static class Rule {
        int id;
        String pkg;
        String role;
    }

    private static class App {
        String pkg;
        String label;
        boolean system;
    }

    public RoutingPage(Activity host, Skin skin) {
        this.host = host;
        this.skin = skin;
    }

    public View build() {
        LinearLayout page = skin.column();
        page.setPadding(skin.dp(22), 0, skin.dp(22), 0);

        LinearLayout everything = skin.card();
        everything.addView(skin.label("Захват всего трафика по умолчанию", skin.text, 17));
        everything.addView(skin.note(
                "Как захватывается и маршрутизируется трафик приложений без своего правила "
                        + "и трафик самой системы"));

        defaultRow = skin.column();
        LinearLayout.LayoutParams rowAt = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        rowAt.topMargin = skin.dp(12);
        everything.addView(defaultRow, rowAt);
        page.addView(everything, skin.gap(14));

        LinearLayout box = skin.card();
        LinearLayout bar = new LinearLayout(host);
        bar.setOrientation(LinearLayout.HORIZONTAL);
        bar.setGravity(Gravity.CENTER_VERTICAL);
        bar.addView(skin.label("Правила", skin.text, 17),
                new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f));

        TextView add = skin.button("Выбрать приложение", skin.good);
        add.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                pickApp();
            }
        });
        bar.addView(add);
        box.addView(bar);

        list = skin.column();
        box.addView(list);
        page.addView(box, skin.gap(14));

        page.addView(skin.note(
                "Для применения правил с маршрутизацией трафика напрямую необходимо "
                        + "переподключиться на главном экране"));

        TextView foot = skin.label("подключение →", skin.muted, 14);
        foot.setGravity(Gravity.CENTER);
        foot.setPadding(0, skin.dp(18), 0, skin.dp(8));
        page.addView(foot);

        ScrollView scroll = new ScrollView(host);
        scroll.setClipChildren(false);
        scroll.setClipToPadding(false);
        page.setClipChildren(false);
        page.setClipToPadding(false);
        scroll.addView(page);
        scroll.setFillViewport(true);
        return scroll;
    }

    public void render() {
        if (!loaded) {
            load();
        }
        if (!defaultRole.equals(shownRole)) {
            shownRole = defaultRole;
            defaultRow.removeAllViews();
            defaultRow.addView(skin.segments(BASE_NAMES, indexIn(BASE_ROLES, defaultRole),
                    new Skin.Pick() {
                        @Override
                        public void at(int index) {
                            defaultRole = BASE_ROLES[index];
                            render();
                            save();
                        }
                    }));
        }
    }

    private void load() {
        if (loading) {
            return;
        }
        loading = true;

        new Thread(new Runnable() {
            @Override
            public void run() {
                String raw = null;
                String problem = null;
                try {
                    raw = Core.client(host).rulesJSON();
                } catch (Exception e) {
                    problem = String.valueOf(e.getMessage());
                }

                final String body = raw;
                final String said = problem;
                host.runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        loading = false;
                        if (said != null) {
                            say(said, true);
                            return;
                        }
                        take(body);
                    }
                });
            }
        }).start();
    }

    private void take(String raw) {
        try {
            JSONObject body = new JSONObject(raw);
            String kept = body.optString("defaultRole", "tunnel");
            boolean stray = !"direct".equals(kept) && !"tunnel".equals(kept);
            defaultRole = stray ? "tunnel" : kept;

            rules.clear();
            JSONArray rows = body.optJSONArray("rules");
            for (int i = 0; rows != null && i < rows.length(); i++) {
                JSONObject row = rows.getJSONObject(i);
                Rule rule = new Rule();
                rule.id = row.optInt("id");
                rule.pkg = row.optString("process");
                rule.role = row.optString("role", "tunnel");
                rules.add(rule);
            }
            loaded = true;
            redraw();
            render();
            if (stray) {
                save();
            }
        } catch (Exception e) {
            say(String.valueOf(e.getMessage()), true);
        }
    }

    private void redraw() {
        list.removeAllViews();
        for (Rule rule : rules) {
            list.addView(row(rule));
        }
    }

    private View row(final Rule rule) {
        LinearLayout block = skin.column();
        block.setPadding(0, skin.dp(16), 0, skin.dp(2));

        LinearLayout line = new LinearLayout(host);
        line.setOrientation(LinearLayout.HORIZONTAL);
        line.setGravity(Gravity.CENTER_VERTICAL);

        LinearLayout names = skin.column();
        names.addView(skin.label(labelOf(rule.pkg), skin.text, 15));
        names.addView(skin.label(rule.pkg, skin.muted, 11));
        line.addView(names, new LinearLayout.LayoutParams(0,
                LinearLayout.LayoutParams.WRAP_CONTENT, 1f));

        TextView remove = skin.label("✕", skin.muted, 18);
        remove.setPadding(skin.dp(14), skin.dp(6), skin.dp(4), skin.dp(6));
        remove.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                drop(rule);
            }
        });
        line.addView(remove);
        block.addView(line);

        final LinearLayout picker = skin.column();
        LinearLayout.LayoutParams pickAt = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        pickAt.topMargin = skin.dp(8);
        block.addView(picker, pickAt);
        fill(picker, rule);

        return block;
    }

    private void fill(final LinearLayout picker, final Rule rule) {
        picker.removeAllViews();
        picker.addView(skin.segments(NAMES, indexOf(rule.role), new Skin.Pick() {
            @Override
            public void at(int index) {
                if (ROLES[index].equals(rule.role)) {
                    return;
                }
                rule.role = ROLES[index];
                fill(picker, rule);
                save();
            }
        }));
    }

    private static int indexOf(String role) {
        return indexIn(ROLES, role);
    }

    private static int indexIn(String[] among, String role) {
        for (int i = 0; i < among.length; i++) {
            if (among[i].equals(role)) {
                return i;
            }
        }
        return among.length - 1;
    }

    private void drop(final Rule rule) {
        new AlertDialog.Builder(host, R.style.RoundDialog)
                .setTitle(labelOf(rule.pkg))
                .setMessage("Убрать правило?")
                .setNegativeButton("Отмена", null)
                .setPositiveButton("Убрать", (dialog, which) -> {
                    rules.remove(rule);
                    redraw();
                    save();
                })
                .show();
    }


    private void pickApp() {
        if (catalogue != null) {
            offer(catalogue);
            return;
        }

        new Thread(new Runnable() {
            @Override
            public void run() {
                final List<App> found = installed();
                host.runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        catalogue = found;
                        offer(found);
                    }
                });
            }
        }).start();
    }

    private void offer(final List<App> apps) {
        LinearLayout wrap = skin.column();
        wrap.setPadding(skin.dp(18), skin.dp(8), skin.dp(18), 0);

        final EditText search = new EditText(host);
        search.setHint("поиск");
        search.setTextColor(skin.text);
        search.setSingleLine(true);
        wrap.addView(search);

        final LinearLayout found = skin.column();
        ScrollView scroll = new ScrollView(host);
        scroll.setClipChildren(false);
        scroll.setClipToPadding(false);
        scroll.addView(found);
        wrap.addView(scroll, new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, skin.dp(380)));

        final AlertDialog dialog = new AlertDialog.Builder(host, R.style.RoundDialog)
                .setTitle("Приложение")
                .setView(wrap)
                .setNegativeButton("Отмена", null)
                .create();

        fill(found, apps, "", dialog);
        search.addTextChangedListener(new TextWatcher() {
            @Override
            public void beforeTextChanged(CharSequence s, int a, int b, int c) {
            }

            @Override
            public void onTextChanged(CharSequence s, int a, int b, int c) {
                fill(found, apps, s.toString(), dialog);
            }

            @Override
            public void afterTextChanged(Editable s) {
            }
        });

        dialog.show();
    }

    private void fill(LinearLayout into, List<App> apps, String needle, final AlertDialog dialog) {
        into.removeAllViews();
        String query = needle.trim().toLowerCase();
        int shown = 0;

        for (final App app : apps) {
            if (!query.isEmpty()
                    && !app.label.toLowerCase().contains(query)
                    && !app.pkg.toLowerCase().contains(query)) {
                continue;
            }
            if (shown++ > 300) {
                break;
            }

            LinearLayout item = skin.column();
            item.setPadding(0, skin.dp(10), 0, skin.dp(10));
            item.addView(skin.label(app.label, skin.text, 15));
            item.addView(skin.label(app.pkg, skin.muted, 11));
            item.setOnClickListener(new View.OnClickListener() {
                @Override
                public void onClick(View v) {
                    dialog.dismiss();
                    adopt(app.pkg);
                }
            });
            into.addView(item);
        }
    }

    private void adopt(String pkg) {
        for (Rule rule : rules) {
            if (rule.pkg.equals(pkg)) {
                say("Уже в списке", false);
                return;
            }
        }

        Rule rule = new Rule();
        rule.pkg = pkg;
        rule.role = "tunnel";
        rules.add(rule);
        redraw();
        save();
    }

    private void save() {
        final String body;
        try {
            JSONArray rows = new JSONArray();
            for (Rule rule : rules) {
                JSONObject row = new JSONObject();
                row.put("id", rule.id);
                row.put("process", rule.pkg);
                row.put("role", rule.role);
                rows.put(row);
            }
            JSONObject payload = new JSONObject();
            payload.put("defaultRole", defaultRole);
            payload.put("rules", rows);
            body = payload.toString();
        } catch (Exception e) {
            say(String.valueOf(e.getMessage()), true);
            return;
        }

        new Thread(new Runnable() {
            @Override
            public void run() {
                String problem = null;
                try {
                    Core.client(host).saveRulesJSON(body);
                } catch (Exception e) {
                    problem = e.getMessage();
                }
                final String said = problem;
                host.runOnUiThread(new Runnable() {
                    @Override
                    public void run() {
                        if (said != null) {
                            say(said, true);
                        }
                        loaded = false;
                        render();
                    }
                });
            }
        }).start();
    }

    private List<App> installed() {
        if (catalogue != null) {
            return catalogue;
        }

        PackageManager packages = host.getPackageManager();
        List<App> out = new ArrayList<>();

        for (PackageInfo info : packages.getInstalledPackages(PackageManager.GET_PERMISSIONS)) {
            if (info.applicationInfo == null || !networked(info)) {
                continue;
            }
            if (info.packageName.equals(host.getPackageName())) {
                continue;
            }

            App app = new App();
            app.pkg = info.packageName;
            app.label = String.valueOf(packages.getApplicationLabel(info.applicationInfo));
            app.system = (info.applicationInfo.flags & ApplicationInfo.FLAG_SYSTEM) != 0;
            out.add(app);
            labels.put(app.pkg, app.label);
        }

        Collections.sort(out, (a, b) -> {
            if (a.system != b.system) {
                return a.system ? 1 : -1;
            }
            return a.label.compareToIgnoreCase(b.label);
        });

        catalogue = out;
        return out;
    }

    private static boolean networked(PackageInfo info) {
        if (info.requestedPermissions == null) {
            return false;
        }
        for (String permission : info.requestedPermissions) {
            if ("android.permission.INTERNET".equals(permission)) {
                return true;
            }
        }
        return false;
    }

    private String labelOf(String pkg) {
        String known = labels.get(pkg);
        if (known != null) {
            return known;
        }
        try {
            PackageManager packages = host.getPackageManager();
            String label = String.valueOf(
                    packages.getApplicationLabel(packages.getApplicationInfo(pkg, 0)));
            labels.put(pkg, label);
            return label;
        } catch (Exception e) {
            return pkg;
        }
    }

    private void say(String text, boolean bad) {
    }


}
