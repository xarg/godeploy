var FU = {
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

FU.Breadcrumbs = {
    'listLogs': 'Logs',
    'viewLog': 'viewLog',
    'runJob': 'Run job',
}
/* Router */
FU.Router = Backbone.Router.extend({
    routes: {
        'listLogs': 'renderListLogs',
        'viewLog/:id': 'renderViewLog',

        'listJobs': 'renderListJobs',
        'runJob/:id': 'renderRunJob', 
    },
    renderListLogs: function () {
        FU.logsView.listLogs();
    },
    renderViewLog: function (id) {
        FU.logsView.viewLog(id);
    },

    renderListJobs: function () {
        FU.jobsView.listJobs();
    },
    renderRunJob: function (id) {
        FU.jobsView.runJob(id);
    },
});

/* Models */
FU.jobModel = Backbone.Model.extend({
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
                            FU.router.navigate('list_contacts', {trigger: true});
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

FU.logModel = Backbone.Model.extend({
    sync: function (method, model, options) {

    }
})

/* Collections */
FU.jobCollection = Backbone.Collection.extend({
    model: FU.jobModel,
    url: "/jobs"
});

FU.logCollection = Backbone.Collection.extend({
    model: FU.logModel,
    url: "/logs"
});


/* Views */
FU.jobsView = Backbone.View.extend({
    el: '#main',
    listTemplate: _.template($('#jobListTemplate').html()),
    runTemplate: _.template($('#runJobTemplate').html()),
    initialize: function () {
        _.bindAll(this, 'listJobs', 'runJob');
    },
    listJobs: function () {
        var self = this;
        FU.jobCollection.fetch({
            success: function (collection, response) {
                self.$el.html(self.listTemplate({jobs: response}));
            }
        });
    },
    runJob: function(id) {
        var self = this;
        self.$el.html(self.runTemplate());
        $.get("/run/" + id, function(data){
            $("#jobBody").text(data);
        });
    }
});

FU.logsView = Backbone.View.extend({
    el: '#main',
    listTemplate: _.template($('#logListTemplate').html()),
    viewTemplate: _.template($('#viewLogTemplate').html()),
    initialize: function () {
        _.bindAll(this, 'listLogs', 'viewLog');
    },
    listLogs: function () {
        var self = this;
        FU.logCollection.fetch({
            success: function (collection, response) {
                self.$el.html(self.listTemplate({logs: response}));
            }
        });
    },
    viewLog: function (id) {
        var self = this;
        FU.logCollection.fetch({
            data: "name=" + id,
            success: function (collection, response) {
                self.$el.html(self.viewTemplate({body: response.body}));
            }
        });
    }
});

$(function () {
    FU.run();
});
