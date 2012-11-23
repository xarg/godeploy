var GD = {
    run: function () {
        // initializing views
        this.jobsView = new this.jobsView();
        this.logsView = new this.logsView();

        // initializing collections
        this.jobCollection = new this.jobCollection();
        this.logCollection = new this.logCollection();

        // init routers
        this.router = new this.Router();
        Backbone.history.start();
        this.router.navigate('listLogs', {trigger: true});
    }
};

/* Router */
GD.Router = Backbone.Router.extend({
    routes: {
        'listLogs': 'renderListLogs',
        'listLogs/:id': 'renderListLogs',
        'viewLog/:id': 'renderViewLog',

        'listJobs': 'renderListJobs',
        'runJob/:id': 'renderRunJob', 
    },
    renderListLogs: function (id) {
        GD.logsView.listLogs(id);
    },
    renderViewLog: function (id) {
        GD.logsView.viewLog(id);
    },

    renderListJobs: function () {
        GD.jobsView.listJobs();
    },
    renderRunJob: function (id) {
        GD.jobsView.runJob(id);
    },
});

/* Models */
GD.jobModel = Backbone.Model.extend({
    sync: function (method, model, options) {
        if (method === 'create' || method === 'update') {
            return $.ajax({
                dataType: 'json',
                url: './addJob',
                data: {
                    id: (this.get('id') || ''), 
                    cmd: (this.get('cmd') || '')
                },
                success: function (data) {
                    $('span.false').html('');
                    if (data.success === true) {
                        if (method === 'update') {
                            GD.router.navigate('list_contacts', {trigger: true});
                        } else {
                            $('form').get(0).reset();
                        }
                    } else {
                        $.each(data.validationError, function () {
                            $('span.' + this.target).html(this.error);
                        });
                    }
                    $('span.success').html(data.msg).removeClass('false').addClass(data.success.toString());
                }
            });
        } else if (method === 'delete') {
            var id = this.get('id');
            return $.getJSON('./deleteJob', { id: id }, function (data) {
                if (data.success === true) {
                    $('#jobsGrid tr[data-id="' + id + '"]').hide('slow');
                } else {
                    alert(data.msg);
                }
            });
        }
    }
});

GD.logModel = Backbone.Model.extend({
    sync: function (method, model, options) {

    }
})

/* Collections */
GD.jobCollection = Backbone.Collection.extend({
    model: GD.jobModel,
    url: "/jobs"
});

GD.logCollection = Backbone.Collection.extend({
    model: GD.logModel,
    url: "/logs"
});


/* Views */
GD.jobsView = Backbone.View.extend({
    el: '#main',
    listTemplate: _.template($('#jobListTemplate').html()),
    runTemplate: _.template($('#runJobTemplate').html()),
    initialize: function () {
        _.bindAll(this, 'listJobs', 'runJob');
    },
    listJobs: function () {
        var self = this;
        GD.jobCollection.fetch({
            success: function (collection, response) {
                self.$el.html(self.listTemplate({jobs: response}));
            }
        });
    },
    runJob: function(id) {
        var self = this;
        self.$el.html(self.runTemplate());
        // run remote command and print the results in the iframe
        // WebSockets? This hack is to easy not to use so no WS fo now.
        var frame = $("<iframe id='jobFrame' style='width: 100%; border: none' src='/run/" + id + "'>")
        $("#jobBody").append(frame);
    }
});

GD.logsView = Backbone.View.extend({
    el: '#main',
    listTemplate: _.template($('#logListTemplate').html()),
    viewTemplate: _.template($('#viewLogTemplate').html()),
    initialize: function () {
        _.bindAll(this, 'listLogs', 'viewLog');
    },
    listLogs: function (id) {
        var self = this;
        var data = "";
        if (id) {
            data = "job=" + id;
        }
        GD.logCollection.fetch({
            data: data,
            success: function (collection, response) {
                self.$el.html(self.listTemplate({logs: response}));
            }
        });
    },
    viewLog: function (id) {
        var self = this;
        GD.logCollection.fetch({
            data: "name=" + id,
            success: function (collection, response) {
                self.$el.html(self.viewTemplate({body: response.body}));
            }
        });
    }
});

$(function () {
    GD.run();
});
